package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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

// Unit: import takes the domain id alone; Read fills status_page_id from the GET.
func TestStatusPageDomainImportAndReadPageID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status-pages/domains/7" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"domain":{"id":7,"statusPageId":42,"hostname":"status.example.com","status":"pending",
			"verifiedAt":null,"createdAt":"c","updatedAt":"u"},"dns":{"txtName":"_uptimeify.status.example.com","txtValue":"tok"}}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	r := &statusPageDomainResource{client: client.New(srv.URL, "wsm_test", "test")}
	var sch resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sch)
	imp := resource.ImportStateResponse{State: tfsdk.State{Schema: sch.Schema}}
	imp.State.Raw = tftypes.NewValue(sch.Schema.Type().TerraformType(ctx), nil)
	r.ImportState(ctx, resource.ImportStateRequest{ID: "7"}, &imp)
	if imp.Diagnostics.HasError() {
		t.Fatal(imp.Diagnostics)
	}
	resp := resource.ReadResponse{State: imp.State}
	r.Read(ctx, resource.ReadRequest{State: imp.State}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got statusPageDomainModel
	resp.State.Get(ctx, &got)
	if got.ID.ValueString() != "7" || got.StatusPageID.ValueInt64() != 42 || got.Hostname.ValueString() != "status.example.com" {
		t.Fatalf("state %+v", got)
	}

	bad := resource.ImportStateResponse{State: imp.State}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "status.example.com"}, &bad)
	if !bad.Diagnostics.HasError() {
		t.Fatal("non-numeric import id accepted")
	}
}

// Unit: create takes status_page_id from the POST answer (domain.statusPageId), not from the plan.
func TestStatusPageDomainCreateReadsPageID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/status-pages/domains" {
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"domain":{"id":7,"statusPageId":43,"hostname":"status.example.com","status":"pending",
			"verifiedAt":null,"createdAt":"c","updatedAt":"u"},"dns":{"txtName":"_uptimeify.status.example.com","txtValue":"tok"}}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	r := &statusPageDomainResource{client: client.New(srv.URL, "wsm_test", "test")}
	var sch resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sch)
	plan := tfsdk.Plan{Schema: sch.Schema}
	if d := plan.Set(ctx, &statusPageDomainModel{ID: types.StringUnknown(), StatusPageID: types.Int64Value(42),
		Hostname: types.StringValue("status.example.com"), Status: types.StringUnknown(), VerifiedAt: types.StringUnknown(),
		VerificationRecord: types.ObjectUnknown(verificationRecordAttrTypes), CreatedAt: types.StringUnknown()}); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got statusPageDomainModel
	resp.State.Get(ctx, &got)
	if got.ID.ValueString() != "7" || got.StatusPageID.ValueInt64() != 43 {
		t.Fatalf("state %+v", got)
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
  description               = "tfacc page"
  include_customer_monitors = true
  hidden_monitors           = [{ type = "dns", id = 999999 }]
  ip_allowlist              = ["203.0.113.0/24", "2001:db8::/32"]
  sms_subscriptions_enabled = true`)
	step2 := statusPageConfig(t, name+"-2", name, `
  visibility                 = "password_protected"
  access_password_wo         = "tfacc-password-1"
  access_password_wo_version = 1
  is_published               = false
  subscription_delay_minutes = 10
  hidden_monitors            = [{ type = "dns", id = 999999 }, { type = "http", id = 999998 }]
  push_subscriptions_enabled = true
  management_type            = "self_service"
  monitor_updates_enabled    = false`)

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
					tfresource.TestCheckResourceAttr(r, "management_type", "managed"),
					tfresource.TestCheckResourceAttr("data.uptimeify_status_page.by_name", "management_type", "managed"),
					tfresource.TestCheckResourceAttr(r, "is_published", "true"),
					tfresource.TestCheckResourceAttr(r, "subscriptions_enabled", "true"),
					tfresource.TestCheckResourceAttr(r, "subscription_delay_minutes", "5"),
					tfresource.TestCheckResourceAttr(r, "has_password", "false"),
					tfresource.TestCheckResourceAttr(r, "include_customer_monitors", "true"),
					tfresource.TestCheckResourceAttr(r, "monitor_updates_enabled", "true"),
					tfresource.TestCheckResourceAttr("data.uptimeify_status_page.by_id", "monitor_updates_enabled", "true"),
					tfresource.TestCheckResourceAttr(r, "hidden_monitors.#", "1"),
					tfresource.TestCheckResourceAttrSet(r, "public_id"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_status_page.by_id", "public_id", r, "public_id"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_status_page.by_slug", "id", r, "id"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_status_page.by_name", "description", r, "description"),
					tfresource.TestCheckResourceAttr("data.uptimeify_status_page.by_name", "hidden_monitors.#", "1"),
					tfresource.TestCheckResourceAttr(r, "ip_allowlist.#", "2"),
					tfresource.TestCheckTypeSetElemAttr(r, "ip_allowlist.*", "2001:db8::/32"),
					tfresource.TestCheckResourceAttr(r, "sms_subscriptions_enabled", "true"),
					tfresource.TestCheckResourceAttr(r, "push_subscriptions_enabled", "false"),
					tfresource.TestCheckResourceAttr("data.uptimeify_status_page.by_id", "ip_allowlist.#", "2"),
					tfresource.TestCheckResourceAttr("data.uptimeify_status_page.by_id", "sms_subscriptions_enabled", "true"),
				),
			},
			{ // update in place: rename, clear description, password protection, monitors
				Config: step2,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSameID(r, &pageID),
					tfresource.TestCheckResourceAttr(r, "name", name+"-2"),
					tfresource.TestCheckNoResourceAttr(r, "description"),
					tfresource.TestCheckResourceAttr(r, "visibility", "password_protected"),
					tfresource.TestCheckResourceAttr(r, "management_type", "self_service"),
					tfresource.TestCheckResourceAttr(r, "monitor_updates_enabled", "false"),
					tfresource.TestCheckResourceAttr(r, "has_password", "true"),
					tfresource.TestCheckResourceAttrSet(r, "access_password_set_at"),
					tfresource.TestCheckNoResourceAttr(r, "access_password_wo"),
					tfresource.TestCheckResourceAttr(r, "is_published", "false"),
					tfresource.TestCheckResourceAttr(r, "subscription_delay_minutes", "10"),
					tfresource.TestCheckResourceAttr(r, "hidden_monitors.#", "2"),
					// removed from the configuration: back to the defaults (no IP restriction, no SMS)
					tfresource.TestCheckResourceAttr(r, "ip_allowlist.#", "0"),
					tfresource.TestCheckResourceAttr(r, "sms_subscriptions_enabled", "false"),
					tfresource.TestCheckResourceAttr(r, "push_subscriptions_enabled", "true"),
					// unset after being set: kept as is, no change
					tfresource.TestCheckResourceAttr(r, "include_customer_monitors", "true"),
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

// A page without customer_id belongs to the organization; attaching and detaching a customer
// moves it in place.
func TestAccStatusPageOrganizationWide(t *testing.T) {
	spAccSkip(t)
	name := fmt.Sprintf("tfacc-spo-%d", time.Now().UnixNano()%1_000_000)
	const r = "uptimeify_status_page.p"
	var pageID string
	orgPage := fmt.Sprintf(`
resource "uptimeify_status_page" "p" {
  name                      = %q
  slug                      = %q
  include_customer_monitors = false
  monitor_updates_enabled   = false
}
`, name, name)
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy:             statusPageGone(&pageID),
		Steps: []tfresource.TestStep{
			{
				Config: orgPage,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSaveID(r, &pageID),
					tfresource.TestCheckNoResourceAttr(r, "customer_id"),
					tfresource.TestCheckNoResourceAttr(r, "customer_name"),
					tfresource.TestCheckResourceAttr(r, "include_customer_monitors", "false"),
					tfresource.TestCheckResourceAttr(r, "monitor_updates_enabled", "false"), // set by the PATCH after create
				),
			},
			{ResourceName: r, ImportState: true, ImportStateVerify: true},
			{
				Config: statusPageConfig(t, name, name, `include_customer_monitors = false`),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSameID(r, &pageID),
					tfresource.TestCheckResourceAttrSet(r, "customer_id"),
					tfresource.TestCheckResourceAttrSet(r, "customer_name"),
					tfresource.TestCheckResourceAttr(r, "monitor_updates_enabled", "true"), // removed: back to the default
				),
			},
			{
				Config: orgPage,
				Check:  tfresource.ComposeAggregateTestCheckFunc(spSameID(r, &pageID), tfresource.TestCheckNoResourceAttr(r, "customer_id")),
			},
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
  custom_title    = "tfacc title"
  custom_css      = ".tfacc { color: red; }"
  status_labels   = { operational = "tfacc ok", major_outage = "tfacc down" }`)
	// the map is the complete list: dropping a state resets it to the default text
	step2b := design(`layout = "cards"
  accent_color    = "#112233"
  history_days    = 14
  show_powered_by = false
  custom_title    = "tfacc title"
  custom_css      = ".tfacc { color: red; }"
  status_labels   = { degraded = "tfacc slow" }`)

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
					tfresource.TestCheckResourceAttr(r, "custom_css", ".tfacc { color: red; }"),
					tfresource.TestCheckResourceAttr(r, "custom_css_stored", ".tfacc{color:red}"),
					tfresource.TestCheckResourceAttr(r, "status_labels.%", "2"),
					tfresource.TestCheckResourceAttr(r, "status_labels.major_outage", "tfacc down"),
				),
			},
			{
				Config: step2b,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(r, "status_labels.%", "1"),
					tfresource.TestCheckResourceAttr(r, "status_labels.degraded", "tfacc slow"),
					func(*terraform.State) error {
						d, err := accClient().GetStatusPageDesign(context.Background(), pageID)
						if err != nil {
							return err
						}
						if len(d.StatusLabels) != 1 || d.StatusLabels["degraded"] != "tfacc slow" {
							return fmt.Errorf("stored statusLabels %v", d.StatusLabels)
						}
						return nil
					},
				),
			},
			// import reads the stored (regenerated) form; custom_css_stored is compared instead
			{ResourceName: r, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"custom_css"}},
			{ // drift: changed outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if _, err := accClient().UpdateStatusPageDesign(context.Background(), pageID, map[string]any{"layout": "minimal"}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             step2b,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: step2b, Check: tfresource.TestCheckResourceAttr(r, "layout", "cards")},
			{ // destroying the design resets it to the defaults, the page stays
				Config: page,
				Check: func(*terraform.State) error {
					d, err := accClient().GetStatusPageDesign(context.Background(), pageID)
					if err != nil {
						return err
					}
					if d.Layout != "statusboard" || !d.ShowPoweredBy || d.HistoryDays != 30 || d.CustomTitle != "" || d.CustomCSS != "" || len(d.StatusLabels) != 0 {
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
			{ // import by domain id; status_page_id is read back
				ResourceName:      r,
				ImportState:       true,
				ImportStateVerify: true,
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

// Acceptance (error path): the sandbox has no DNS, so verify fails. The activation must fail once
// (no retry), name the TXT record and its value in the message, and leave nothing in state.
func TestAccStatusPageDomainActivationVerifyFails(t *testing.T) {
	spAccSkip(t)
	name := fmt.Sprintf("tfacc-spact-%d", time.Now().UnixNano()%1_000_000)
	var pageID, domainID string
	base := statusPageConfig(t, name, name, "") + fmt.Sprintf(`
resource "uptimeify_status_page_domain" "d" {
  status_page_id = tonumber(uptimeify_status_page.p.id)
  hostname       = %q
}
`, name+".example.com")
	// Filled from the domain's state in step 1; Terraform wraps long diagnostics at spaces.
	expect := regexp.MustCompile(`unset`)
	saveRecord := func(s *terraform.State) error {
		a := s.RootModule().Resources["uptimeify_status_page_domain.d"].Primary.Attributes
		*expect = *regexp.MustCompile(`(?s)Verifying status page domain failed.*TXT\s+record\s+"` +
			regexp.QuoteMeta(a["verification_record.name"]) + `"\s+with\s+the\s+value\s+"` +
			regexp.QuoteMeta(a["verification_record.value"]) + `"`)
		return nil
	}

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			id, _ := strconv.ParseInt(domainID, 10, 64)
			if _, _, err := accClient().GetStatusPageDomain(context.Background(), id); !client.IsNotFound(err) {
				return fmt.Errorf("domain %s still exists (err=%v)", domainID, err)
			}
			return statusPageGone(&pageID)(nil)
		},
		Steps: []tfresource.TestStep{
			{
				Config: base,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSaveID("uptimeify_status_page.p", &pageID),
					spSaveID("uptimeify_status_page_domain.d", &domainID),
					tfresource.TestCheckResourceAttrSet("uptimeify_status_page_domain.d", "verification_record.name"),
					tfresource.TestCheckResourceAttrSet("uptimeify_status_page_domain.d", "verification_record.value"),
					saveRecord,
				),
			},
			{
				Config: base + `
resource "uptimeify_status_page_domain_activation" "a" {
  domain_id = tonumber(uptimeify_status_page_domain.d.id)
}
`,
				ExpectError: expect,
			},
			{ // the failed create stored nothing
				Config: base,
				Check: func(s *terraform.State) error {
					if r := s.RootModule().Resources["uptimeify_status_page_domain_activation.a"]; r != nil {
						return fmt.Errorf("activation in state after failed verify: %v", r.Primary.Attributes)
					}
					return nil
				},
			},
		},
	})
}

// sso_members_only needs verified SSO of the organization; without it the API refuses the page.
func TestAccStatusPageSSOMembersOnlyNeedsSSO(t *testing.T) {
	spAccSkip(t)
	name := fmt.Sprintf("tfacc-spsso-%d", time.Now().UnixNano()%1_000_000)
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		Steps: []tfresource.TestStep{{
			Config:      statusPageConfig(t, name, name, `visibility = "sso_members_only"`),
			ExpectError: regexp.MustCompile(`ssoNotConfigured`),
		}},
	})
}

func TestAccStatusPageAudienceGroups(t *testing.T) {
	spAccSkip(t)
	name := fmt.Sprintf("tfacc-spa-%d", time.Now().UnixNano()%1_000_000)
	const r = "uptimeify_status_page_audience_groups.a"
	var pageID string
	config := func(groups string) string {
		return statusPageConfig(t, name, name, "") + fmt.Sprintf(`
resource "uptimeify_status_page_component" "intern" {
  status_page_id = tonumber(uptimeify_status_page.p.id)
  source_kind    = "manual"
  name           = "tfacc intern"
}

resource "uptimeify_status_page_component" "partner" {
  status_page_id = tonumber(uptimeify_status_page.p.id)
  source_kind    = "manual"
  name           = "tfacc partner"
}

resource "uptimeify_status_page_audience_groups" "a" {
  status_page_id = tonumber(uptimeify_status_page.p.id)
  groups         = %s
}
`, groups)
	}
	step1 := config(`[{
    name          = "Staff"
    email_domains = ["tfacc.example"]
    component_ids = [uptimeify_status_page_component.intern.component_id]
  }]`)
	step2 := config(`[
    { name = "Staff", email_domains = ["tfacc.example", "tfacc-2.example"], component_ids = [uptimeify_status_page_component.intern.component_id] },
    { name = "Partner", email_domains = ["partner.example"], component_ids = [uptimeify_status_page_component.partner.component_id, uptimeify_status_page_component.intern.component_id] },
  ]`)
	groupsOnServer := func(want int) tfresource.TestCheckFunc {
		return func(*terraform.State) error {
			p, err := accClient().GetStatusPage(context.Background(), pageID)
			if err != nil {
				return err
			}
			if len(p.AudienceGroups) != want {
				return fmt.Errorf("want %d audience groups on the page, got %+v", want, p.AudienceGroups)
			}
			return nil
		}
	}
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
					tfresource.TestCheckResourceAttr(r, "groups.#", "1"),
					tfresource.TestCheckResourceAttr(r, "groups.0.name", "Staff"),
					tfresource.TestCheckTypeSetElemAttrPair(r, "groups.0.component_ids.*", "uptimeify_status_page_component.intern", "component_id"),
					groupsOnServer(1),
				),
			},
			{
				Config: step2,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(r, "groups.#", "2"),
					tfresource.TestCheckResourceAttr(r, "groups.0.email_domains.#", "2"),
					tfresource.TestCheckResourceAttr(r, "groups.1.name", "Partner"),
					tfresource.TestCheckResourceAttr(r, "groups.1.component_ids.#", "2"),
					groupsOnServer(2),
				),
			},
			{ResourceName: r, ImportState: true, ImportStateVerify: true},
			{ // drift: groups removed outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().UpdateStatusPage(context.Background(), pageID, map[string]any{"audienceGroups": []any{}}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             step2,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: step2, Check: groupsOnServer(2)},
			{ // destroying the resource removes the groups, page and components stay
				Config: statusPageConfig(t, name, name, ""),
				Check:  groupsOnServer(0),
			},
		},
	})
}

// Unit: the CIDR validator accepts exactly the form the API stores unchanged.
func TestCIDRValidator(t *testing.T) {
	for in, ok := range map[string]bool{
		"203.0.113.0/24": true, "203.0.113.5/32": true, "2001:db8::/32": true,
		"203.0.113.5": false, "2001:DB8::/32": false, "203.0.113.0/33": false, "nonsense": false, "fe80::1%eth0/64": false,
	} {
		var resp validator.StringResponse
		cidrValidator{}.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringValue(in)}, &resp)
		if resp.Diagnostics.HasError() == ok {
			t.Errorf("%q: accepted=%v, want %v", in, !resp.Diagnostics.HasError(), ok)
		}
	}
}

// Unit: status_labels is the complete list; every state it leaves out goes to the API as null.
func TestStatusLabelsPatch(t *testing.T) {
	m := types.MapValueMust(types.StringType, map[string]attr.Value{"partial_outage": types.StringValue("Teilweise")})
	got := statusLabelsPatch(context.Background(), m)
	if len(got) != 5 || got["partialOutage"] != "Teilweise" || got["operational"] != nil || got["majorOutage"] != nil {
		t.Fatalf("patch %v", got)
	}
	back := statusLabelsFromAPI(map[string]string{"partialOutage": "Teilweise", "unknownKey": "x"})
	if len(back.Elements()) != 1 || back.Elements()["partial_outage"].(types.String).ValueString() != "Teilweise" {
		t.Fatalf("from API %v", back)
	}
}

// Unit: the create endpoint does not take monitorUpdatesEnabled; false goes in a PATCH right after.
func TestStatusPageCreateMonitorUpdatesOff(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		calls = append(calls, r.Method+" "+r.URL.Path+" "+string(b))
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"statusPage":{"id":5,"slug":"p"}}`))
		case http.MethodPatch:
			_, _ = w.Write([]byte(`{"statusPage":{"id":5}}`))
		default:
			_, _ = w.Write([]byte(`{"id":5,"publicId":"u","organizationId":2,"customerId":null,"name":"P","slug":"p",
				"visibility":"public","managementType":"managed","isPublished":true,"subscriptionsEnabled":true,
				"subscriptionDelayMinutes":5,"includeCustomerMonitors":false,"monitorUpdatesEnabled":false,"hiddenMonitors":[],
				"ipAllowlist":[],"hasPassword":false,"createdAt":"c","updatedAt":"u"}`))
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	r := &statusPageResource{client: client.New(srv.URL, "wsm_test", "test")}
	var sch resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sch)
	m := statusPageResourceModel{statusPageModel: statusPageModel{ID: types.StringUnknown(), PublicID: types.StringUnknown(),
		OrganizationID: types.Int64Unknown(), CustomerID: types.Int64Null(), CustomerName: types.StringUnknown(),
		Name: types.StringValue("P"), Slug: types.StringUnknown(), Description: types.StringNull(),
		Visibility: types.StringValue("public"), ManagementType: types.StringUnknown(), IsPublished: types.BoolValue(true),
		SubscriptionsEnabled: types.BoolValue(true), SubscriptionDelayMinutes: types.Int64Value(5),
		IncludeCustomerMonitors: types.BoolUnknown(), MonitorUpdatesEnabled: types.BoolValue(false),
		HiddenMonitors:          types.SetValueMust(types.ObjectType{AttrTypes: monitorRefAttrTypes}, nil),
		SMSSubscriptionsEnabled: types.BoolValue(false), PushSubscriptionsEnabled: types.BoolValue(false),
		IPAllowlist: types.SetValueMust(types.StringType, nil), HasPassword: types.BoolUnknown(),
		AccessPasswordSetAt: types.StringUnknown(), CreatedAt: types.StringUnknown(), UpdatedAt: types.StringUnknown()},
		AccessPasswordWO: types.StringNull(), AccessPasswordWOVersion: types.Int64Null()}
	plan := tfsdk.Plan{Schema: sch.Schema}
	if d := plan.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan, Config: tfsdk.Config{Schema: sch.Schema, Raw: plan.Raw}}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if len(calls) != 3 || strings.Contains(calls[0], "monitorUpdatesEnabled") ||
		calls[1] != `PATCH /api/status-pages/5 {"monitorUpdatesEnabled":false}` {
		t.Fatalf("calls %q", calls)
	}
	var got statusPageResourceModel
	resp.State.Get(ctx, &got)
	if got.MonitorUpdatesEnabled.ValueBool() {
		t.Fatal("monitor_updates_enabled not read back as false")
	}
}
