package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Acceptance against the real app: PUT runs the issuer's discovery with the app's SSRF guard, so
// the issuer must be a public HTTPS IdP the app can reach. UPTIMEIFY_ACC_SSO_ISSUER, default
// https://accounts.google.com (only its discovery document is fetched; no login happens). The TXT
// record cannot exist, so the verification resource must fail with the record to publish.
func TestAccSSOOIDC(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	accPreCheck(t)
	issuer := firstNonEmpty(os.Getenv("UPTIMEIFY_ACC_SSO_ISSUER"), "https://accounts.google.com")
	domain := fmt.Sprintf("tfacc-%d.example.com", time.Now().Unix()%1_000_000)
	const r = "uptimeify_sso_oidc.s"
	cfg := func(role string, secretVersion int, verify bool) string {
		v := ""
		if verify {
			v = `resource "uptimeify_sso_domain_verification" "v" { domain = uptimeify_sso_oidc.s.domain }`
		}
		return fmt.Sprintf(`
terraform {
  required_providers {
    uptimeify = { source = "uptimeify/uptimeify" }
  }
}

resource "uptimeify_sso_oidc" "s" {
  issuer                   = %q
  client_id                = "tfacc-client"
  client_secret_wo         = "tfacc-secret-%d"
  client_secret_wo_version = %[2]d
  domain                   = %q
  default_role             = %q
}
%s
`, issuer, secretVersion, domain, role, v)
	}
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			s, err := accClient().GetSSO(context.Background())
			if err != nil {
				return err
			}
			if s.Configured {
				return fmt.Errorf("SSO still configured (domain %s)", s.Domain)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: cfg("editor", 1, false),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(r, "id", "sso"),
					tfresource.TestCheckResourceAttr(r, "issuer", issuer),
					tfresource.TestCheckResourceAttr(r, "domain", domain),
					tfresource.TestCheckResourceAttr(r, "default_role", "editor"),
					tfresource.TestCheckResourceAttr(r, "enforce_sso", "false"),
					tfresource.TestCheckResourceAttr(r, "domain_verified", "false"),
					tfresource.TestCheckResourceAttr(r, "can_enforce", "false"),
					tfresource.TestCheckResourceAttr(r, "scopes.#", "3"),
					tfresource.TestMatchResourceAttr(r, "redirect_uri", regexp.MustCompile(`/api/auth/sso/callback/org-\d+$`)),
					tfresource.TestCheckResourceAttr(r, "verification_record.name", "_uptimeify-sso."+domain),
					tfresource.TestMatchResourceAttr(r, "verification_record.value", regexp.MustCompile(`^uptimeify-sso=.+`)),
					tfresource.TestCheckNoResourceAttr(r, "client_secret_wo"),
				),
			},
			{ // in place: new role, resent secret; the TXT value stays (same domain and issuer)
				Config: cfg("readonly", 2, false),
				Check:  tfresource.TestCheckResourceAttr(r, "default_role", "readonly"),
			},
			{
				ResourceName:            r,
				ImportState:             true,
				ImportStateId:           "sso",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"client_secret_wo_version"},
			},
			{
				Config:      cfg("readonly", 2, true),
				ExpectError: regexp.MustCompile(`Publish the DNS TXT record "_uptimeify-sso\.`),
			},
		},
	})
}

// Fake API, real CLI (needs TF_ACC_TERRAFORM_PATH, as in scripts/acceptance.sh): a visible TXT
// record verifies once, the next plan is empty, and a reset verification (new issuer) plans the
// verification again.
func TestAccSSODomainVerificationFakeAPI(t *testing.T) {
	if os.Getenv("TF_ACC_TERRAFORM_PATH") == "" {
		t.Skip("TF_ACC_TERRAFORM_PATH not set")
	}
	if os.Getenv("TF_ACC_PROVIDER_NAMESPACE") == "" {
		t.Setenv("TF_ACC_PROVIDER_NAMESPACE", "uptimeify")
	}
	var mu sync.Mutex
	view := map[string]any{"configured": false, "redirectUri": "https://x/api/auth/sso/callback/org-1", "scopes": []string{"openid", "email", "profile"}}
	verifyCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method + " " + r.URL.Path {
		case "GET /api/organization/sso":
		case "PUT /api/organization/sso":
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if view["issuer"] != b["issuer"] { // new issuer: verification reset, new token
				view["domainVerified"] = false
				view["txtRecordValue"] = fmt.Sprintf("uptimeify-sso=tok-%v", b["issuer"])
			}
			for _, k := range []string{"issuer", "clientId", "domain", "defaultRole", "enforceSso"} {
				view[k] = b[k]
			}
			view["configured"], view["type"], view["canEnforce"], view["hasClientSecret"] = true, "oidc", false, true
			view["txtRecordName"] = fmt.Sprintf("_uptimeify-sso.%v", b["domain"])
		case "POST /api/organization/sso/verify-domain":
			verifyCalls++
			view["domainVerified"] = true
			_ = json.NewEncoder(w).Encode(map[string]any{"verified": true, "expected": view["txtRecordValue"], "found": []any{view["txtRecordValue"]}})
			return
		case "DELETE /api/organization/sso":
			view = map[string]any{"configured": false}
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(view)
	}))
	defer srv.Close()
	cfg := func(issuer string) string {
		return fmt.Sprintf(`
terraform {
  required_providers {
    uptimeify = { source = "uptimeify/uptimeify" }
  }
}
provider "uptimeify" {
  endpoint  = %q
  api_token = "wsm_test"
}
resource "uptimeify_sso_oidc" "s" {
  issuer           = %q
  client_id        = "c"
  client_secret_wo = "s"
  domain           = "example.com"
}
resource "uptimeify_sso_domain_verification" "v" { domain = uptimeify_sso_oidc.s.domain }
`, srv.URL, issuer)
	}
	calls := func(want int) tfresource.TestCheckFunc {
		return func(*terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if verifyCalls != want {
				return fmt.Errorf("verify-domain called %d times, want %d", verifyCalls, want)
			}
			return nil
		}
	}
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testAccProviders,
		Steps: []tfresource.TestStep{
			{Config: cfg("https://idp.example.com"), Check: tfresource.ComposeAggregateTestCheckFunc(
				calls(1),
				tfresource.TestCheckResourceAttr("uptimeify_sso_domain_verification.v", "id", "sso"),
			)},
			{Config: cfg("https://idp.example.com"), PlanOnly: true},
			{Config: cfg("https://idp2.example.com"), ExpectNonEmptyPlan: true, Check: tfresource.ComposeAggregateTestCheckFunc(
				tfresource.TestCheckResourceAttr("uptimeify_sso_oidc.s", "verification_record.value", "uptimeify-sso=tok-https://idp2.example.com"),
			)},
			// The reset verification is only visible after the apply: the next one verifies again.
			{Config: cfg("https://idp2.example.com"), Check: calls(2)},
		},
	})
}
