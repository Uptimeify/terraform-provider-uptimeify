package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// Unit: create PATCHes only configured design fields, update only changed ones.
func TestStatusPageDesignPatch(t *testing.T) {
	plan := statusPageDesignModel{Layout: types.StringValue("cards"), ShowHistory: types.BoolUnknown(),
		HistoryDays: types.Int64Value(14), AccentColor: types.StringNull()}
	if got := designPatch(&plan, nil); len(got) != 2 || got["layout"] != "cards" || got["historyDays"] != int64(14) {
		t.Fatalf("create patch: %v", got)
	}
	state := plan
	state.Layout = types.StringValue("classic")
	if got := designPatch(&plan, &state); len(got) != 1 || got["layout"] != "cards" {
		t.Fatalf("update patch: %v", got)
	}
}

// Unit: activation runs verify then activate; a failed verify names the TXT record and stores nothing.
func TestStatusPageDomainActivationCreate(t *testing.T) {
	verifyOK := false
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/status-pages/domains/verify":
			if !verifyOK {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"statusMessage":"TXT verification failed (token not found)","data":{"code":"domainVerificationFailed"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"verified":true,"status":"verified","domain":{"id":7}}`))
		case "/api/status-pages/domains/activate":
			_, _ = w.Write([]byte(`{"activated":true,"domain":{"id":7,"status":"active"}}`))
		case "/api/status-pages/domains/7":
			_, _ = w.Write([]byte(`{"domain":{"id":7,"status":"pending"},"dns":{"txtName":"_uptimeify.s.example.com","txtValue":"tok"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	r := &statusPageDomainActivationResource{client: client.New(srv.URL, "wsm_test", "test")}
	var sch resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sch)
	run := func() resource.CreateResponse {
		plan := tfsdk.Plan{Schema: sch.Schema}
		plan.Set(ctx, &statusPageDomainActivationModel{ID: types.StringUnknown(), DomainID: types.Int64Value(7), Status: types.StringUnknown()})
		resp := resource.CreateResponse{State: tfsdk.State{Schema: sch.Schema}}
		r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
		return resp
	}

	resp := run()
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), `"_uptimeify.s.example.com" with the value "tok"`) {
		t.Fatalf("want a verify error naming the TXT record, got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("state set after failed verify")
	}

	verifyOK, calls = true, nil
	resp = run()
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got statusPageDomainActivationModel
	resp.State.Get(ctx, &got)
	if got.Status.ValueString() != "active" || got.ID.ValueString() != "7" {
		t.Fatalf("state %+v", got)
	}
	if strings.Join(calls, ",") != "POST /api/status-pages/domains/verify,POST /api/status-pages/domains/activate" {
		t.Fatalf("calls %v", calls)
	}
}

// ---- acceptance ------------------------------------------------------------------------------
// Needs TF_ACC=1, UPTIMEIFY_ENDPOINT, UPTIMEIFY_API_TOKEN (org admin); customer as in TestAccWebsite.

// statusPageConfig is the customer lookup plus uptimeify_status_page.p with slug = slug.
func statusPageConfig(t *testing.T, name, slug, extra string) string {
	return accCustomerConfig(t) + fmt.Sprintf(`
resource "uptimeify_status_page" "p" {
  customer_id = tonumber(data.uptimeify_customer.c.id)
  name        = %q
  slug        = %q
  %s
}
`, name, slug, extra)
}

func statusPageGone(id *string) func(*terraform.State) error {
	return func(*terraform.State) error {
		if _, err := accClient().GetStatusPage(context.Background(), *id); !client.IsNotFound(err) {
			return fmt.Errorf("status page %s still exists (err=%v)", *id, err)
		}
		return nil
	}
}

func spSaveID(res string, into *string) tfresource.TestCheckFunc {
	return func(s *terraform.State) error {
		*into = s.RootModule().Resources[res].Primary.ID
		return nil
	}
}

func spSameID(res string, want *string) tfresource.TestCheckFunc {
	return func(s *terraform.State) error {
		if got := s.RootModule().Resources[res].Primary.ID; got != *want {
			return fmt.Errorf("%s was replaced: %s -> %s", res, *want, got)
		}
		return nil
	}
}

func spAccSkip(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	accPreCheck(t)
}

func TestAccStatusPage(t *testing.T) {
	spAccSkip(t)
	name := fmt.Sprintf("tfacc-sp-%d", time.Now().UnixNano()%1_000_000)
	const r = "uptimeify_status_page.p"
	var pageID string
	step1 := statusPageConfig(t, name, name, `
  description     = "tfacc page"
  hidden_monitors = [{ type = "dns", id = 999999 }]`)
	step2 := statusPageConfig(t, name+"-2", name, `
  visibility                 = "password_protected"
  access_password_wo         = "tfacc-password-1"
  access_password_wo_version = 1
  is_published               = false
  subscription_delay_minutes = 10
  hidden_monitors            = [{ type = "dns", id = 999999 }, { type = "http", id = 999998 }]`)

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy:             statusPageGone(&pageID),
		Steps: []tfresource.TestStep{
			{ // create + data sources by id, slug and name
				Config: step1 + `
data "uptimeify_status_page" "by_id"   { id = uptimeify_status_page.p.id }
data "uptimeify_status_page" "by_slug" { slug = uptimeify_status_page.p.slug }
data "uptimeify_status_page" "by_name" { name = uptimeify_status_page.p.name }
`,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSaveID(r, &pageID),
					tfresource.TestCheckResourceAttr(r, "slug", name),
					tfresource.TestCheckResourceAttr(r, "visibility", "public"),
					tfresource.TestCheckResourceAttr(r, "is_published", "true"),
					tfresource.TestCheckResourceAttr(r, "subscriptions_enabled", "true"),
					tfresource.TestCheckResourceAttr(r, "subscription_delay_minutes", "5"),
					tfresource.TestCheckResourceAttr(r, "has_password", "false"),
					tfresource.TestCheckResourceAttr(r, "hidden_monitors.#", "1"),
					tfresource.TestCheckResourceAttrSet(r, "public_id"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_status_page.by_id", "public_id", r, "public_id"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_status_page.by_slug", "id", r, "id"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_status_page.by_name", "description", r, "description"),
					tfresource.TestCheckResourceAttr("data.uptimeify_status_page.by_name", "hidden_monitors.#", "1"),
				),
			},
			{ // update in place: rename, clear description, password protection, monitors
				Config: step2,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSameID(r, &pageID),
					tfresource.TestCheckResourceAttr(r, "name", name+"-2"),
					tfresource.TestCheckNoResourceAttr(r, "description"),
					tfresource.TestCheckResourceAttr(r, "visibility", "password_protected"),
					tfresource.TestCheckResourceAttr(r, "has_password", "true"),
					tfresource.TestCheckResourceAttrSet(r, "access_password_set_at"),
					tfresource.TestCheckNoResourceAttr(r, "access_password_wo"),
					tfresource.TestCheckResourceAttr(r, "is_published", "false"),
					tfresource.TestCheckResourceAttr(r, "subscription_delay_minutes", "10"),
					tfresource.TestCheckResourceAttr(r, "hidden_monitors.#", "2"),
				),
			},
			{ResourceName: r, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"access_password_wo_version"}},
			{ // drift: deleted outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteStatusPage(context.Background(), pageID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             step2,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: step2, Check: spSaveID(r, &pageID)},
		},
	})
}

func TestAccStatusPageDesign(t *testing.T) {
	spAccSkip(t)
	name := fmt.Sprintf("tfacc-spd-%d", time.Now().UnixNano()%1_000_000)
	const r = "uptimeify_status_page_design.d"
	var pageID string
	page := statusPageConfig(t, name, name, "")
	design := func(attrs string) string {
		return page + fmt.Sprintf(`
resource "uptimeify_status_page_design" "d" {
  status_page_id = tonumber(uptimeify_status_page.p.id)
  %s
}
`, attrs)
	}
	step1 := design(`layout = "classic"
  accent_color = "#112233"
  history_days = 14`)
	step2 := design(`layout = "cards"
  accent_color    = "#112233"
  history_days    = 14
  show_powered_by = false
  custom_title    = "tfacc title"`)

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy:             statusPageGone(&pageID),
		Steps: []tfresource.TestStep{
			{
				Config: step1,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSaveID("uptimeify_status_page.p", &pageID),
					tfresource.TestCheckResourceAttrPair(r, "id", "uptimeify_status_page.p", "id"),
					tfresource.TestCheckResourceAttr(r, "layout", "classic"),
					tfresource.TestCheckResourceAttr(r, "accent_color", "#112233"),
					tfresource.TestCheckResourceAttr(r, "history_days", "14"),
					tfresource.TestCheckResourceAttr(r, "show_powered_by", "true"),
					tfresource.TestCheckResourceAttr(r, "incident_limit", "5"),
					tfresource.TestCheckResourceAttr(r, "color_scheme", "auto"),
				),
			},
			{
				Config: step2,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(r, "layout", "cards"),
					tfresource.TestCheckResourceAttr(r, "show_powered_by", "false"),
					tfresource.TestCheckResourceAttr(r, "custom_title", "tfacc title"),
					tfresource.TestCheckResourceAttr(r, "history_days", "14"),
				),
			},
			{ResourceName: r, ImportState: true, ImportStateVerify: true},
			{ // drift: changed outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if _, err := accClient().UpdateStatusPageDesign(context.Background(), pageID, map[string]any{"layout": "minimal"}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             step2,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: step2, Check: tfresource.TestCheckResourceAttr(r, "layout", "cards")},
			{ // destroying the design resets it to the defaults, the page stays
				Config: page,
				Check: func(*terraform.State) error {
					d, err := accClient().GetStatusPageDesign(context.Background(), pageID)
					if err != nil {
						return err
					}
					if d.Layout != "statusboard" || !d.ShowPoweredBy || d.HistoryDays != 30 || d.CustomTitle != "" {
						return fmt.Errorf("design not reset: %+v", d)
					}
					return nil
				},
			},
		},
	})
}

func TestAccStatusPageDomain(t *testing.T) {
	spAccSkip(t)
	name := fmt.Sprintf("tfacc-spdom-%d", time.Now().UnixNano()%1_000_000)
	const r = "uptimeify_status_page_domain.d"
	var pageID, domainID string
	domain := func(host string) string {
		return statusPageConfig(t, name, name, "") + fmt.Sprintf(`
resource "uptimeify_status_page_domain" "d" {
  status_page_id = tonumber(uptimeify_status_page.p.id)
  hostname       = %q
}
`, host)
	}
	host := name + ".example.com"
	domainGone := func(*terraform.State) error {
		id, _ := strconv.ParseInt(domainID, 10, 64)
		if _, _, err := accClient().GetStatusPageDomain(context.Background(), id); !client.IsNotFound(err) {
			return fmt.Errorf("domain %s still exists (err=%v)", domainID, err)
		}
		return statusPageGone(&pageID)(nil)
	}

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy:             domainGone,
		Steps: []tfresource.TestStep{
			{
				Config: domain(host),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSaveID("uptimeify_status_page.p", &pageID),
					spSaveID(r, &domainID),
					tfresource.TestCheckResourceAttr(r, "hostname", host),
					tfresource.TestCheckResourceAttr(r, "status", "pending"),
					tfresource.TestCheckResourceAttrSet(r, "verification_record.name"),
					tfresource.TestCheckResourceAttrSet(r, "verification_record.value"),
					tfresource.TestCheckNoResourceAttr(r, "verified_at"),
				),
			},
			{ // import by status page id
				ResourceName:      r,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(*terraform.State) (string, error) { return pageID, nil },
			},
			{ // drift: deleted outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					id, _ := strconv.ParseInt(domainID, 10, 64)
					if err := accClient().DeleteStatusPageDomain(context.Background(), id); err != nil {
						t.Fatal(err)
					}
				},
				Config:             domain(host),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: domain(host), Check: spSaveID(r, &domainID)},
			{ // a new hostname replaces the domain
				Config: domain("www." + host),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						if got := s.RootModule().Resources[r].Primary.ID; got == domainID {
							return fmt.Errorf("domain %s was not replaced", got)
						}
						return nil
					},
					spSaveID(r, &domainID),
					tfresource.TestCheckResourceAttr(r, "hostname", "www."+host),
				),
			},
		},
	})
}

func TestAccImStatuspageRule(t *testing.T) {
	team := accImTeam(t) // skips without TF_ACC
	name := fmt.Sprintf("tfacc-sprule-%d", time.Now().UnixNano()%1_000_000)
	const r = "uptimeify_im_statuspage_rule.r"
	var ruleID string
	rule := func(attrs string) string {
		return statusPageConfig(t, name, name, "") + fmt.Sprintf(`
resource "uptimeify_im_statuspage_rule" "r" {
  team_id        = %d
  status_page_id = tonumber(uptimeify_status_page.p.id)
  %s
}
`, team, attrs)
	}
	step2 := rule(`min_severity = "sev1"
  target_state = "warning"
  auto_resolve = false`)

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			if _, err := accClient().GetImStatuspageRule(context.Background(), ruleID); !client.IsNotFound(err) {
				return fmt.Errorf("rule %s still exists (err=%v)", ruleID, err)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: rule("") + `
data "uptimeify_im_statuspage_rule" "by_id" { id = uptimeify_im_statuspage_rule.r.id }
`,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSaveID(r, &ruleID),
					tfresource.TestCheckResourceAttr(r, "team_id", strconv.FormatInt(team, 10)),
					tfresource.TestCheckResourceAttrPair(r, "status_page_id", "uptimeify_status_page.p", "id"),
					tfresource.TestCheckResourceAttr(r, "min_severity", "sev2"),
					tfresource.TestCheckResourceAttr(r, "target_state", "degraded"),
					tfresource.TestCheckResourceAttr(r, "auto_resolve", "true"),
					tfresource.TestCheckResourceAttrSet(r, "organization_id"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_im_statuspage_rule.by_id", "min_severity", r, "min_severity"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_im_statuspage_rule.by_id", "status_page_id", r, "status_page_id"),
				),
			},
			{
				Config: step2,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSameID(r, &ruleID),
					tfresource.TestCheckResourceAttr(r, "min_severity", "sev1"),
					tfresource.TestCheckResourceAttr(r, "target_state", "warning"),
					tfresource.TestCheckResourceAttr(r, "auto_resolve", "false"),
				),
			},
			{ResourceName: r, ImportState: true, ImportStateVerify: true},
			{ // drift: deleted outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteImStatuspageRule(context.Background(), ruleID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             step2,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: step2, Check: spSaveID(r, &ruleID)},
		},
	})
}
