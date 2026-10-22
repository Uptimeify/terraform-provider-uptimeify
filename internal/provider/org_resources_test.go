package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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

// Unit: every schema of this package is valid for the framework.
func TestOrgSchemasValid(t *testing.T) {
	ctx := context.Background()
	for _, f := range []func() resource.Resource{NewOrgUserResource, NewOrgSettingsResource, NewWhitelabelDomainResource,
		NewReportResource, NewSMTPSettingsResource} {
		var resp resource.SchemaResponse
		f().Schema(ctx, resource.SchemaRequest{}, &resp)
		if d := resp.Schema.ValidateImplementation(ctx); d.HasError() {
			t.Fatal(d)
		}
	}
}

// Unit: the data sources store the API structs as is, so tags and schema must match.
func TestOrgDataSourcesDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/organization":
			_, _ = w.Write([]byte(`{"id":2,"publicId":"p","name":"Org","status":"active","deletionDueAt":null,"emailLanguage":"de",
				"billingEmail":null,"billingCycle":null,"paymentMethod":null,"activeDiscount":null,
				"defaultNotificationChannels":{"email":true,"sms":false,"webhook":false,"integrations":true},
				"defaultNotificationTargets":null,"defaultNotificationRules":null,"requireMfaAdmins":true,"city":"Berlin",
				"smsOverageSafetyCapCentsMonthly":500}`))
		case "/api/package-configs":
			_, _ = w.Write([]byte(`[{"id":1,"packageType":"essential","displayName":null,"maxUrls":10,"allowedCheckCountryCodes":["DE"],
				"enableSslCheck":true,"maxSelfServiceUrls":0,"flappingIncidentCount":3,"usedByCustomerCount":4}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	c := client.New(srv.URL, "wsm_test", "test")

	for _, ds := range []datasource.DataSourceWithConfigure{&organizationDataSource{client: c}, &packageConfigsDataSource{client: c}} {
		var sch datasource.SchemaResponse
		ds.Schema(ctx, datasource.SchemaRequest{}, &sch)
		resp := datasource.ReadResponse{State: tfsdk.State{Schema: sch.Schema}}
		ds.Read(ctx, datasource.ReadRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
	}
}

// Unit: the user PATCH sends only what changed, and both names together.
func TestOrgUserUpdateSendsChangedFieldsOnly(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		got = string(b)
		_, _ = w.Write([]byte(`{"success":true,"user":{"id":"u1","email":"a@example.test","name":"Ann B","firstName":"Ann",
			"lastName":"B","image":null,"role":"editor","isActive":true,"createdAt":"t","customerAccess":[]}}`))
	}))
	defer srv.Close()
	ctx := context.Background()
	r := &orgUserResource{client: client.New(srv.URL, "wsm_test", "test")}
	var sch resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sch)

	base := orgUserModel{ID: types.StringValue("u1"), Email: types.StringValue("a@example.test"), FirstName: types.StringValue("Ann"),
		LastName: types.StringValue("A"), Name: types.StringValue("Ann A"), Role: types.StringValue("editor"), IsActive: types.BoolValue(true),
		CustomerIDs: types.SetValueMust(types.Int64Type, nil), Language: types.StringNull(), CreatedAt: types.StringValue("t")}
	planned := base
	planned.LastName = types.StringValue("B")
	state, plan := tfsdk.State{Schema: sch.Schema}, tfsdk.Plan{Schema: sch.Schema}
	state.Set(ctx, &base)
	plan.Set(ctx, &planned)
	resp := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got != `{"firstName":"Ann","lastName":"B"}` {
		t.Fatalf("PATCH body %s", got)
	}

	planned = base
	planned.Language = types.StringValue("da")
	plan.Set(ctx, &planned)
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got != `{"language":"da"}` {
		t.Fatalf("PATCH body %s", got)
	}
}

// Unit: language is always read back (drift detection); null means the organization language.
func TestOrgUserFromAPILanguage(t *testing.T) {
	en, da := "en", "da"
	var m orgUserModel
	m.fromAPI(&client.UserDetail{ID: "u1", Language: &en})
	if m.Language.ValueString() != "en" {
		t.Fatalf("language %s", m.Language)
	}
	m.fromAPI(&client.UserDetail{ID: "u1", Language: &da})
	if m.Language.ValueString() != "da" {
		t.Fatalf("language %s, want da read back", m.Language)
	}
	m.fromAPI(&client.UserDetail{ID: "u1"})
	if !m.Language.IsNull() {
		t.Fatalf("language %s, want null", m.Language)
	}
}

// orgAccStart skips without TF_ACC (like tfresource.Test) before the test calls the API itself.
func orgAccStart(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	accPreCheck(t)
}

func accSuffix() string { return strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 10) }

// Acceptance: invites tfacc-user-*@example.test (the mail lands in Mailpit), changes role and
// customer access, deletes the login on destroy.
func TestAccOrgUser(t *testing.T) {
	orgAccStart(t)
	email := "tfacc-user-" + accSuffix() + "@example.test"
	var userID string
	cfg := func(role, last, extra string) string {
		return accCustomerConfig(t) + fmt.Sprintf(`
resource "uptimeify_user" "u" {
  email      = %q
  first_name = "Tfacc"
  last_name  = %q
  role       = %q
  language   = "en"
  %s
}
`, email, last, role, extra)
	}
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			if _, err := accClient().GetUser(context.Background(), userID); !client.IsNotFound(err) {
				return fmt.Errorf("user %s still exists (err=%v)", userID, err)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: cfg("editor", "One", ""),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_user.u", "email", email),
					tfresource.TestCheckResourceAttr("uptimeify_user.u", "role", "editor"),
					tfresource.TestCheckResourceAttr("uptimeify_user.u", "name", "Tfacc One"),
					tfresource.TestCheckResourceAttr("uptimeify_user.u", "is_active", "true"),
					tfresource.TestCheckResourceAttr("uptimeify_user.u", "customer_ids.#", "0"),
					func(s *terraform.State) error {
						userID = s.RootModule().Resources["uptimeify_user.u"].Primary.ID
						return nil
					},
				),
			},
			{ // readonly login limited to one customer, deactivated
				Config: cfg("readonly", "Two", "is_active = false\n  customer_ids = [tonumber(data.uptimeify_customer.c.id)]"),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_user.u", "role", "readonly"),
					tfresource.TestCheckResourceAttr("uptimeify_user.u", "name", "Tfacc Two"),
					tfresource.TestCheckResourceAttr("uptimeify_user.u", "is_active", "false"),
					tfresource.TestCheckResourceAttr("uptimeify_user.u", "customer_ids.#", "1"),
					tfresource.TestCheckTypeSetElemAttrPair("uptimeify_user.u", "customer_ids.*", "data.uptimeify_customer.c", "id"),
					func(s *terraform.State) error {
						if got := s.RootModule().Resources["uptimeify_user.u"].Primary.ID; got != userID {
							return fmt.Errorf("user was replaced: %s -> %s", userID, got)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "uptimeify_user.u",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // drift: deleted outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteUser(context.Background(), userID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             cfg("editor", "One", ""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: cfg("editor", "One", ""),
				Check: func(s *terraform.State) error {
					userID = s.RootModule().Resources["uptimeify_user.u"].Primary.ID
					return nil
				},
			},
		},
	})
}

// Acceptance: Org 2 is shared. The test only touches company_name and city, restates the
// current name and MFA switch (never sent: unchanged), and restores the original values.
func TestAccOrgSettings(t *testing.T) {
	orgAccStart(t)
	orig, err := accClient().GetOrganization(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	restore := func() error {
		accPreCheck(t)
		deref := func(s *string) string { // the API has no null for these; "" is the closest
			if s == nil {
				return ""
			}
			return *s
		}
		_, err := accClient().UpdateOrganization(context.Background(), map[string]any{
			"companyName": deref(orig.CompanyName), "city": deref(orig.City)})
		return err
	}
	mfa := ""
	if orig.RequireMfaAdmins != nil {
		mfa = fmt.Sprintf("require_mfa_admins = %t", *orig.RequireMfaAdmins)
	}
	sfx := accSuffix()
	res := func(company, city string) string {
		return imProviderBlock() + fmt.Sprintf(`
resource "uptimeify_organization_settings" "o" {
  name         = %q
  company_name = %q
  city         = %q
  %s
}
`, orig.Name, company, city, mfa)
	}
	cfg := func(company, city string) string {
		return res(company, city) + `
data "uptimeify_organization" "o" {
  depends_on = [uptimeify_organization_settings.o]
}
data "uptimeify_package_configs" "all" {}
`
	}
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			if _, err := accClient().GetOrganization(context.Background()); err != nil {
				return fmt.Errorf("organization gone after destroy: %v", err)
			}
			return restore()
		},
		Steps: []tfresource.TestStep{
			{
				Config: cfg("tfacc-co-"+sfx, "tfacc-city-"+sfx),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_organization_settings.o", "id", strconv.FormatInt(orig.ID, 10)),
					tfresource.TestCheckResourceAttr("uptimeify_organization_settings.o", "company_name", "tfacc-co-"+sfx),
					tfresource.TestCheckResourceAttr("data.uptimeify_organization.o", "city", "tfacc-city-"+sfx),
					tfresource.TestCheckResourceAttr("data.uptimeify_organization.o", "name", orig.Name),
					tfresource.TestCheckResourceAttrSet("data.uptimeify_package_configs.all", "package_configs.#"),
				),
			},
			{
				Config: cfg("tfacc-co2-"+sfx, "tfacc-city2-"+sfx),
				Check:  tfresource.TestCheckResourceAttr("uptimeify_organization_settings.o", "city", "tfacc-city2-"+sfx),
			},
			{
				ResourceName:      "uptimeify_organization_settings.o",
				ImportState:       true,
				ImportStateId:     "org",
				ImportStateVerify: true,
			},
			{ // drift: changed outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if _, err := accClient().UpdateOrganization(context.Background(), map[string]any{"city": "tfacc-drift-" + sfx}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             res("tfacc-co2-"+sfx, "tfacc-city2-"+sfx),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: cfg("tfacc-co2-"+sfx, "tfacc-city2-"+sfx),
				Check:  tfresource.TestCheckResourceAttr("uptimeify_organization_settings.o", "city", "tfacc-city2-"+sfx),
			},
		},
	})
}

func TestAccWhitelabelDomain(t *testing.T) {
	orgAccStart(t)
	sfx := accSuffix()
	var domainID string
	cfg := func(host string) string {
		return imProviderBlock() + fmt.Sprintf(`
resource "uptimeify_whitelabel_domain" "d" {
  hostname = %q
}
`, host)
	}
	host1, host2 := "tfacc-"+sfx+".example.test", "tfacc-"+sfx+"-b.example.test"
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			if _, err := accClient().GetWhitelabelDomain(context.Background(), domainID); !client.IsNotFound(err) {
				return fmt.Errorf("domain %s still exists (err=%v)", domainID, err)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: cfg(host1),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_whitelabel_domain.d", "status", "pending"),
					tfresource.TestCheckResourceAttrSet("uptimeify_whitelabel_domain.d", "dns_txt_name"),
					tfresource.TestCheckResourceAttrSet("uptimeify_whitelabel_domain.d", "dns_txt_value"),
					func(s *terraform.State) error {
						domainID = s.RootModule().Resources["uptimeify_whitelabel_domain.d"].Primary.ID
						return nil
					},
				),
			},
			{ // a new hostname is a new domain
				Config: cfg(host2),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_whitelabel_domain.d", "hostname", host2),
					func(s *terraform.State) error {
						got := s.RootModule().Resources["uptimeify_whitelabel_domain.d"].Primary.ID
						if got == domainID {
							return fmt.Errorf("hostname change did not replace the domain")
						}
						if _, err := accClient().GetWhitelabelDomain(context.Background(), domainID); !client.IsNotFound(err) {
							return fmt.Errorf("old domain %s still exists (err=%v)", domainID, err)
						}
						domainID = got
						return nil
					},
				),
			},
			{ResourceName: "uptimeify_whitelabel_domain.d", ImportState: true, ImportStateVerify: true},
			{
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteWhitelabelDomain(context.Background(), domainID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             cfg(host2),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: cfg(host2),
				Check: func(s *terraform.State) error {
					domainID = s.RootModule().Resources["uptimeify_whitelabel_domain.d"].Primary.ID
					return nil
				},
			},
		},
	})
}

func TestAccReport(t *testing.T) {
	orgAccStart(t)
	name := "tfacc-report-" + accSuffix()
	var reportID string
	weekly := `
  frequency        = "weekly"
  weekday          = 1
  format           = "email"
  recipient_emails = ["tfacc-report@example.test"]
`
	monthly := `
  frequency    = "monthly"
  day_of_month = 5
  send_hour    = 7
  monitor_types = ["website", "dns"]
  sections = {
    fleet_summary = true, worst_performers = false, per_site_table = true
    incident_log = true, ssl_expiry = false, group_by_customer = false
  }
  recipient_emails = ["tfacc-report@example.test", "tfacc-report2@example.test"]
`
	cfg := func(n, body string) string {
		return imProviderBlock() + fmt.Sprintf(`
resource "uptimeify_report" "r" {
  name = %q
  %s
}
`, n, body)
	}
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			if _, err := accClient().GetReport(context.Background(), reportID); !client.IsNotFound(err) {
				return fmt.Errorf("report %s still exists (err=%v)", reportID, err)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: cfg(name, weekly),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_report.r", "frequency", "weekly"),
					tfresource.TestCheckResourceAttr("uptimeify_report.r", "enabled", "true"),
					tfresource.TestCheckResourceAttr("uptimeify_report.r", "timezone", "Europe/Berlin"),
					tfresource.TestCheckResourceAttr("uptimeify_report.r", "monitor_types.#", "10"),
					func(s *terraform.State) error {
						reportID = s.RootModule().Resources["uptimeify_report.r"].Primary.ID
						return nil
					},
				),
			},
			{
				Config: cfg(name+"-2", monthly),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_report.r", "name", name+"-2"),
					tfresource.TestCheckResourceAttr("uptimeify_report.r", "day_of_month", "5"),
					tfresource.TestCheckNoResourceAttr("uptimeify_report.r", "weekday"),
					tfresource.TestCheckResourceAttr("uptimeify_report.r", "sections.worst_performers", "false"),
					tfresource.TestCheckResourceAttr("uptimeify_report.r", "monitor_types.#", "2"),
					tfresource.TestCheckResourceAttr("uptimeify_report.r", "recipient_emails.#", "2"),
					func(s *terraform.State) error {
						if got := s.RootModule().Resources["uptimeify_report.r"].Primary.ID; got != reportID {
							return fmt.Errorf("report was replaced: %s -> %s", reportID, got)
						}
						return nil
					},
				),
			},
			{ResourceName: "uptimeify_report.r", ImportState: true, ImportStateVerify: true},
			{
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteReport(context.Background(), reportID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             cfg(name+"-2", monthly),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: cfg(name+"-2", monthly),
				Check: func(s *terraform.State) error {
					reportID = s.RootModule().Resources["uptimeify_report.r"].Primary.ID
					return nil
				},
			},
		},
	})
}

// Acceptance: runs against its own organization (UPTIMEIFY_ACC_TOKEN_SMTP, an org-admin token of an
// organization without its own SMTP server), because the sandbox's main organization already has
// one and its real configuration is never replaced. The server checks a new host with
// resolvePublicHostnameTarget: a public IP literal needs no DNS (UPTIMEIFY_ACC_SMTP_HOST, default
// 1.1.1.1). Mail of that organization fails while the test runs.
func TestAccSMTPSettings(t *testing.T) {
	orgAccStart(t)
	token := os.Getenv("UPTIMEIFY_ACC_TOKEN_SMTP")
	if token == "" {
		t.Skip("UPTIMEIFY_ACC_TOKEN_SMTP not set: needs an org-admin token of an organization without its own SMTP server")
	}
	smtpClient := func() *client.Client {
		accPreCheck(t)
		return client.New(os.Getenv("UPTIMEIFY_ENDPOINT"), token, "test")
	}
	cur, err := smtpClient().GetSMTP(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cur.Host != nil {
		t.Skipf("organization already has its own SMTP server (%s); not touching it", *cur.Host)
	}
	host := os.Getenv("UPTIMEIFY_ACC_SMTP_HOST")
	if host == "" {
		host = "1.1.1.1"
	}
	cfg := func(fromName string, ver int) string {
		return imProviderBlock() + fmt.Sprintf(`
provider "uptimeify" {
  api_token = %q
}

resource "uptimeify_smtp_settings" "s" {
  host                = %q
  port                = 587
  username            = "tfacc"
  password_wo         = "tfacc-secret-%d"
  password_wo_version = %d
  tls_mode            = "starttls"
  from_name           = %q
  from_email          = "tfacc-smtp@example.test"
}
`, token, host, ver, ver, fromName)
	}
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			s, err := smtpClient().GetSMTP(context.Background())
			if err != nil {
				return err
			}
			if s.Host != nil || s.HasPassword {
				return fmt.Errorf("SMTP still configured after destroy: host=%v password=%t", s.Host, s.HasPassword)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: cfg("tfacc", 1),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_smtp_settings.s", "has_password", "true"),
					tfresource.TestCheckNoResourceAttr("uptimeify_smtp_settings.s", "password_wo"),
				),
			},
			{
				Config: cfg("tfacc-2", 2),
				Check:  tfresource.TestCheckResourceAttr("uptimeify_smtp_settings.s", "from_name", "tfacc-2"),
			},
			{
				ResourceName:            "uptimeify_smtp_settings.s",
				ImportState:             true,
				ImportStateId:           "smtp",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password_wo_version"},
			},
			{ // drift: switched back to platform sending outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					// Immer ueber die Test-Org: das Org-2-Token schaltete am 21.10. den SMTP von Org 2 ab.
					if _, err := smtpClient().UpdateSMTP(context.Background(), map[string]any{"disable": true}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             cfg("tfacc-2", 2),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: cfg("tfacc-2", 2)},
		},
	})
}
