package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

func TestWebsiteSchemasValid(t *testing.T) {
	ctx := context.Background()
	var rs resource.SchemaResponse
	(&websiteResource{}).Schema(ctx, resource.SchemaRequest{}, &rs)
	if d := rs.Schema.ValidateImplementation(ctx); d.HasError() {
		t.Fatal(d)
	}
	for _, ds := range []datasource.DataSource{NewWebsiteDataSource(), NewCustomerDataSource()} {
		var s datasource.SchemaResponse
		ds.Schema(ctx, datasource.SchemaRequest{}, &s)
		if d := s.Schema.ValidateImplementation(ctx); d.HasError() {
			t.Fatal(d)
		}
	}
}

// Unit: the PATCH mode choice. Partial-mode fields go out alone; anything else switches to the
// full mode with every attribute, because that mode resets what it is not sent.
func TestDropDeadChecks(t *testing.T) {
	plan := websiteModel{
		CheckKeywordEnabled: types.BoolValue(true), SearchTerm: types.StringNull(),
		CheckSizeEnabled: types.BoolValue(true), MinPageSize: types.Int64Null(), MaxPageSize: types.Int64Value(9),
	}
	cfg := websiteModel{CheckKeywordEnabled: types.BoolNull(), CheckSizeEnabled: types.BoolNull()}
	if !dropDeadChecks(&plan, &cfg) || plan.CheckKeywordEnabled.ValueBool() || !plan.CheckSizeEnabled.ValueBool() {
		t.Fatalf("keyword without term must plan off, size with a limit stays: %+v", plan)
	}
	plan.CheckKeywordEnabled = types.BoolValue(true)
	cfg.CheckKeywordEnabled = types.BoolValue(true)
	if dropDeadChecks(&plan, &cfg) || !plan.CheckKeywordEnabled.ValueBool() {
		t.Fatal("an explicit switch is the user's call; the server answers it")
	}
}

func TestWebsiteUpdateBody(t *testing.T) {
	base := websiteModel{
		CustomerID: types.Int64Value(7), Name: types.StringValue("a"), URL: types.StringValue("https://1.1.1.1/"),
		MonitoringType: types.StringValue("combined"), Status: types.StringValue("active"),
		CheckInterval: types.Int64Value(30), SearchTerm: types.StringNull(), HTTPMethod: types.StringValue("GET"),
		ConnectHost: types.StringValue("1.0.0.1"), ConnectPort: types.Int64Null(),
		AllowedCheckCountryCodes: types.SetNull(types.StringType), SSLErrorDays: types.Int64Value(0),
	}

	plan := base
	plan.Name = types.StringValue("b")
	plan.CheckInterval = types.Int64Value(5)
	body, full := websiteUpdateBody(&plan, &base, false)
	if full || len(body) != 2 || body["name"] != "b" || body["checkInterval"] != int64(5) {
		t.Fatalf("partial: full=%v body=%v", full, body)
	}

	plan = base
	plan.ConnectPort = types.Int64Value(8443)
	body, _ = websiteUpdateBody(&plan, &base, false)
	if body["connectHost"] != "1.0.0.1" || body["connectPort"] != int64(8443) {
		t.Fatalf("connect port without host: %v", body)
	}

	plan = base
	plan.HTTPMethod = types.StringValue("POST")
	body, full = websiteUpdateBody(&plan, &base, false)
	if !full || body["customerId"] != int64(7) || body["url"] != "https://1.1.1.1/" || body["status"] != "active" ||
		body["sslErrorDays"] != int64(0) || body["httpMethod"] != "POST" {
		t.Fatalf("full: full=%v body=%v", full, body)
	}
	if v, ok := body["searchTerm"]; !ok || v != nil {
		t.Fatalf("full mode must send null to keep a cleared search term cleared: %v", body)
	}

	if _, full = websiteUpdateBody(&base, &base, true); !full {
		t.Fatal("a secret change needs the full mode")
	}

	// After a full update: re-apply only the partial-mode values the server coerced.
	got := base
	got.SSLErrorDays = types.Int64Value(3)
	if fix := partialBody(&base, &got); len(fix) != 1 || fix["sslErrorDays"] != int64(0) {
		t.Fatalf("fixup: %v", fix)
	}
}

// Acceptance: needs TF_ACC=1, UPTIMEIFY_ENDPOINT, UPTIMEIFY_API_TOKEN (org admin). The customer is
// UPTIMEIFY_ACC_CUSTOMER_NAME, else the first one of GET /api/customers. The monitored URL is
// UPTIMEIFY_ACC_WEBSITE_URL, default an IP literal: create and update resolve the host and reject
// non-public answers, and the sandbox has no DNS to the internet.
func TestAccWebsite(t *testing.T) {
	// The configs below look the customer up over the API, so stop before that without TF_ACC.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	accPreCheck(t)
	name := fmt.Sprintf("tfacc-web-%d", time.Now().Unix()%1_000_000)
	url := firstNonEmpty(os.Getenv("UPTIMEIFY_ACC_WEBSITE_URL"), "https://1.1.1.1/")
	var httpID, hbID, hbToken string
	saveIDs := func(s *terraform.State) error {
		httpID = s.RootModule().Resources["uptimeify_website.http"].Primary.ID
		hbID = s.RootModule().Resources["uptimeify_website.hb"].Primary.ID
		hbToken = s.RootModule().Resources["uptimeify_website.hb"].Primary.Attributes["heartbeat_token"]
		return nil
	}
	rotated := func(s *terraform.State) error {
		hb := s.RootModule().Resources["uptimeify_website.hb"].Primary.Attributes
		if hb["heartbeat_token"] == "" || hb["heartbeat_token"] == hbToken {
			return fmt.Errorf("heartbeat token not rotated: %q -> %q", hbToken, hb["heartbeat_token"])
		}
		if hb["url"] != "heartbeat://"+hb["heartbeat_token"] {
			return fmt.Errorf("heartbeat url %q does not carry the new token", hb["url"])
		}
		return nil
	}
	sameID := func(s *terraform.State) error {
		if got := s.RootModule().Resources["uptimeify_website.http"].Primary.ID; got != httpID {
			return fmt.Errorf("website was replaced: %s -> %s", httpID, got)
		}
		return nil
	}
	const r = "uptimeify_website.http"

	step1 := websiteStep{name: name, url: url, interval: 10, codes: "200", countries: `["DE"]`, method: "POST", headersVer: 1,
		checks: `check_keyword_enabled = true
  search_term           = "tfacc"`}
	step2 := step1 // partial mode only
	step2.name, step2.interval, step2.codes = name+"-2", 15, "200,204"
	step2.hbRotation = "r1"
	step3 := step2 // full mode: method, countries, new headers, basic auth
	step3.method, step3.countries, step3.headersVer, step3.basicAuth = "GET", `["DE", "NL"]`, 2, true
	// Keyword check and search term removed in the same full update (DeepL 29.10.2026).
	step3.checks = `check_expected_response_enabled = true
  expected_response_match_type    = "contains"
  expected_response_value         = "tfacc"`

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			for _, id := range []string{httpID, hbID} {
				if _, err := accClient().GetWebsite(context.Background(), id); !client.IsNotFound(err) {
					return fmt.Errorf("website %s still exists (err=%v)", id, err)
				}
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: step1.config(t),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					saveIDs,
					tfresource.TestCheckResourceAttr(r, "name", name),
					tfresource.TestCheckResourceAttr(r, "url", url),
					tfresource.TestCheckResourceAttr(r, "status", "inactive"),
					tfresource.TestCheckResourceAttr(r, "check_interval", "10"),
					tfresource.TestCheckResourceAttr(r, "timeout_seconds", "10"),
					tfresource.TestCheckResourceAttr(r, "expected_status_codes", "200"),
					tfresource.TestCheckResourceAttr(r, "http_method", "POST"),
					tfresource.TestCheckResourceAttr(r, "allowed_check_country_codes.#", "1"),
					tfresource.TestCheckResourceAttr(r, "connect_host", "1.0.0.1"),
					tfresource.TestCheckResourceAttr(r, "connect_port", "443"),
					tfresource.TestCheckResourceAttr(r, "check_keyword_enabled", "true"),
					tfresource.TestCheckResourceAttr(r, "search_term", "tfacc"),
					tfresource.TestCheckResourceAttr(r, "has_custom_headers", "true"),
					tfresource.TestCheckResourceAttr(r, "has_request_body", "true"),
					tfresource.TestCheckNoResourceAttr(r, "custom_headers_wo"),
					tfresource.TestCheckNoResourceAttr(r, "request_body_wo"),
					tfresource.TestCheckResourceAttrPair(r, "customer_id", "data.uptimeify_customer.c", "id"),
					tfresource.TestCheckResourceAttr("uptimeify_website.hb", "monitoring_type", "heartbeat"),
					tfresource.TestCheckResourceAttr("uptimeify_website.hb", "heartbeat_grace_period_minutes", "10"),
					tfresource.TestCheckResourceAttrSet("uptimeify_website.hb", "heartbeat_token"),
					func(s *terraform.State) error {
						hb := s.RootModule().Resources["uptimeify_website.hb"].Primary.Attributes
						if !strings.HasPrefix(hb["url"], "heartbeat://") {
							return fmt.Errorf("heartbeat url %q", hb["url"])
						}
						return nil
					},
				),
			},
			{ // partial PATCH mode; heartbeat token rotation
				Config: step2.config(t) + `
data "uptimeify_website" "by_id" { id = uptimeify_website.http.public_id }
`,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					sameID,
					rotated,
					tfresource.TestCheckResourceAttr(r, "name", name+"-2"),
					tfresource.TestCheckResourceAttr(r, "check_interval", "15"),
					tfresource.TestCheckResourceAttr(r, "expected_status_codes", "200,204"),
					tfresource.TestCheckResourceAttr(r, "http_method", "POST"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_website.by_id", "name", r, "name"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_website.by_id", "connect_host", r, "connect_host"),
					tfresource.TestCheckResourceAttr("data.uptimeify_website.by_id", "has_custom_headers", "true"),
				),
			},
			{ // full PATCH mode; untouched fields (interval, status codes, timeout, status, ssl_error_days) must survive
				Config: step3.config(t),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(r, "check_keyword_enabled", "false"),
					tfresource.TestCheckNoResourceAttr(r, "search_term"),
					tfresource.TestCheckResourceAttr(r, "check_expected_response_enabled", "true"),
					tfresource.TestCheckResourceAttr(r, "expected_response_value", "tfacc"),
					sameID,
					tfresource.TestCheckResourceAttr(r, "http_method", "GET"),
					tfresource.TestCheckResourceAttr(r, "allowed_check_country_codes.#", "2"),
					tfresource.TestCheckResourceAttr(r, "auth_mode", "basic"),
					tfresource.TestCheckResourceAttr(r, "basic_auth_username", "tfacc"),
					tfresource.TestCheckResourceAttr(r, "check_interval", "15"),
					tfresource.TestCheckResourceAttr(r, "expected_status_codes", "200,204"),
					tfresource.TestCheckResourceAttr(r, "timeout_seconds", "10"),
					tfresource.TestCheckResourceAttr(r, "status", "inactive"),
					tfresource.TestCheckResourceAttr(r, "ssl_error_days", "0"),
					tfresource.TestCheckResourceAttr(r, "has_custom_headers", "true"),
				),
			},
			{
				ResourceName:            r,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"custom_headers_wo_version", "request_body_wo_version", "auth_wo_version"},
			},
			{ // drift: deleted outside Terraform -> plan not empty
				PreConfig: func() {
					if err := accClient().DeleteWebsite(context.Background(), httpID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             step3.config(t),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // recreate, then the framework destroys and CheckDestroy verifies
				Config: step3.config(t),
				Check:  saveIDs,
			},
		},
	})
}

type websiteStep struct {
	name, url, codes, countries, method string
	interval, headersVer                int
	basicAuth                           bool
	hbRotation, checks                  string
}

func (s websiteStep) config(t *testing.T) string {
	auth := ""
	if s.basicAuth {
		auth = `
  auth_mode              = "basic"
  basic_auth_username    = "tfacc"
  basic_auth_password_wo = "tfacc-secret"
  auth_wo_version        = 1`
	}
	rotation := ""
	if s.hbRotation != "" {
		rotation = fmt.Sprintf("heartbeat_token_rotation = %q", s.hbRotation)
	}
	return accCustomerConfig(t) + fmt.Sprintf(`
resource "uptimeify_website" "http" {
  customer_id                 = tonumber(data.uptimeify_customer.c.id)
  name                        = %[1]q
  url                         = %[2]q
  status                      = "inactive"
  check_interval              = %[3]d
  timeout_seconds             = 10
  expected_status_codes       = %[5]q
  http_method                 = %[4]q
  allowed_check_country_codes = %[6]s
  connect_host                = "1.0.0.1"
  connect_port                = 443

  custom_headers_wo = [
    { name = "X-Tfacc", value = "v%[7]d" },
    { name = "X-Tfacc-Step", value = "%[7]d" },
  ]
  custom_headers_wo_version = %[7]d
  request_body_wo           = "{\"tfacc\":true}"
  request_body_wo_version   = 1
  %[10]s
%[8]s
}

resource "uptimeify_website" "hb" {
  customer_id                    = tonumber(data.uptimeify_customer.c.id)
  name                           = "%[1]s-hb"
  monitoring_type                = "heartbeat"
  status                         = "inactive"
  check_interval                 = 60
  heartbeat_grace_period_minutes = 10
  %[9]s
}
`, s.name, s.url, s.interval, s.method, s.codes, s.countries, s.headersVer, auth, rotation, s.checks)
}

// accCustomerConfig is the provider block plus data.uptimeify_customer.c: by name from
// UPTIMEIFY_ACC_CUSTOMER_NAME, else by the id of the first customer the token sees.
func accCustomerConfig(t *testing.T) string {
	lookup := fmt.Sprintf("name = %q", os.Getenv("UPTIMEIFY_ACC_CUSTOMER_NAME"))
	if os.Getenv("UPTIMEIFY_ACC_CUSTOMER_NAME") == "" {
		cs, err := accClient().ListCustomers(context.Background())
		if err != nil || len(cs) == 0 {
			t.Fatalf("no customer for the acceptance test (err=%v); set UPTIMEIFY_ACC_CUSTOMER_NAME", err)
		}
		lookup = fmt.Sprintf("id = \"%d\"", cs[0].ID)
	}
	return fmt.Sprintf(`
terraform {
  required_providers {
    uptimeify = { source = "uptimeify/uptimeify" }
  }
}

data "uptimeify_customer" "c" { %s }
`, lookup)
}

// Unit: a check the package forbids comes back off. Create must say so clearly and still keep
// the created monitor in state.
func TestWebsiteCreateClampedCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		row := `{"id":5,"publicId":"p","customerId":7,"name":"a","url":"https://1.1.1.1/","monitoringType":"combined",` +
			`"status":"active","checkInterval":30,"timeoutSeconds":30,"authMode":"none","checkKeywordEnabled":false,` +
			`"createdAt":"t","updatedAt":"t"}`
		_, _ = w.Write([]byte(row))
	}))
	defer srv.Close()

	ctx := context.Background()
	res := &websiteResource{client: client.New(srv.URL, "wsm_test", "test")}
	var sch resource.SchemaResponse
	res.Schema(ctx, resource.SchemaRequest{}, &sch)

	headerType := sch.Schema.Attributes["custom_headers_wo"].GetType().(types.ListType).ElemType
	m := websiteResourceModel{CustomHeadersWO: types.ListNull(headerType)}
	m.AllowedCheckCountryCodes = types.SetNull(types.StringType)
	m.CustomerID, m.Name, m.URL = types.Int64Value(7), types.StringValue("a"), types.StringValue("https://1.1.1.1/")
	m.CheckKeywordEnabled = types.BoolValue(true)
	plan := tfsdk.Plan{Schema: sch.Schema}
	if d := plan.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: plan, Config: tfsdk.Config{Schema: sch.Schema, Raw: plan.Raw}}, &resp)

	if resp.Diagnostics.ErrorsCount() != 1 || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "check_keyword_enabled = true is not allowed") {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	var got websiteResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.ID.ValueString() != "5" || got.CheckKeywordEnabled.ValueBool() {
		t.Fatalf("state: id=%s keyword=%v", got.ID, got.CheckKeywordEnabled)
	}
}
