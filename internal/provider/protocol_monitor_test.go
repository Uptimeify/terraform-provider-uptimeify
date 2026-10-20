package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// Unit: every resource and data source schema passes the framework's own validation
// (write-only rules, defaults on computed attributes, data source projection).
func TestProviderSchemasValid(t *testing.T) {
	srv, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range resp.Diagnostics {
		t.Errorf("%s: %s", d.Summary, d.Detail)
	}
	for _, name := range []string{"dns", "tcp", "icmp", "smtp", "ssh", "ftp", "imap_pop"} {
		if resp.ResourceSchemas["uptimeify_"+name+"_monitor"] == nil || resp.DataSourceSchemas["uptimeify_"+name+"_monitor"] == nil {
			t.Errorf("uptimeify_%s_monitor not registered", name)
		}
	}
	for _, a := range resp.DataSourceSchemas["uptimeify_ssh_monitor"].Block.Attributes {
		if a.Name == "password_wo" || a.Name == "secrets_wo_version" {
			t.Errorf("data source exposes %s", a.Name)
		}
	}
}

// Unit: a monitor deleted outside Terraform (404) leaves the state.
func TestMonitorReadNotFoundRemovesResource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"statusCode":404,"statusMessage":"DNS monitor not found"}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	r := NewDNSMonitorResource().(*monitorResource[dnsMonitorModel, *dnsMonitorModel])
	r.client = client.New(srv.URL, "wsm_test", "test")
	var sch resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sch)

	m := dnsMonitorModel{
		RecordTypes:    types.ListNull(types.StringType),
		ExpectedValues: types.MapNull(types.ListType{ElemType: types.StringType}),
	}
	m.ID = types.StringValue("7")
	m.AllowedCheckCountryCodes = types.SetNull(types.StringType)
	state := tfsdk.State{Schema: sch.Schema}
	if d := state.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("resource still in state after 404")
	}
}

// Unit: PATCH bodies carry only changes; a write-only secret only goes out with a new version
// or when the monitor leaves tcp mode.
func TestSMTPPatchSecrets(t *testing.T) {
	ctx := context.Background()
	mk := func(user string, version int64, mode string) *smtpMonitorModel {
		m := &smtpMonitorModel{
			Secure: types.BoolValue(false), IgnoreTLS: types.BoolValue(false), RequireTLS: types.BoolValue(false),
			AuthUser: types.StringValue(user), AuthPassWO: types.StringNull(), AuthPassWOVersion: types.Int64Value(version),
		}
		m.CheckMode = types.StringValue(mode)
		m.Port = types.Int64Null()
		m.Name = types.StringValue("x")
		m.AllowedCheckCountryCodes = types.SetNull(types.StringType)
		return m
	}
	cfg := mk("u", 1, "protocol")
	cfg.AuthPassWO = types.StringValue("s3cret")
	body := func(plan, prior *smtpMonitorModel) string {
		b := newBody(false)
		plan.putBase(ctx, b, &prior.monitorBase)
		plan.put(ctx, b, cfg, prior)
		out, _ := json.Marshal(b.body)
		return string(out)
	}

	if got := body(mk("u", 1, "protocol"), mk("u", 1, "protocol")); got != `{}` {
		t.Errorf("no change: %s", got)
	}
	if got, want := body(mk("v", 1, "protocol"), mk("u", 1, "protocol")),
		`{"smtpConfig":{"auth":{"user":"v"},"ignoreTls":false,"requireTls":false,"secure":false}}`; got != want {
		t.Errorf("user change:\n got %s\nwant %s", got, want)
	}
	if got, want := body(mk("u", 2, "protocol"), mk("u", 1, "protocol")),
		`{"smtpConfig":{"auth":{"pass":"s3cret","user":"u"},"ignoreTls":false,"requireTls":false,"secure":false}}`; got != want {
		t.Errorf("version bump:\n got %s\nwant %s", got, want)
	}
	if got, want := body(mk("u", 1, "protocol"), mk("u", 1, "tcp")),
		`{"checkMode":"protocol","smtpConfig":{"auth":{"pass":"s3cret","user":"u"},"ignoreTls":false,"requireTls":false,"secure":false}}`; got != want {
		t.Errorf("leave tcp mode:\n got %s\nwant %s", got, want)
	}
	if got := body(mk("u", 1, "tcp"), mk("u", 1, "protocol")); got != `{"checkMode":"tcp"}` {
		t.Errorf("enter tcp mode: %s", got)
	}
}

func TestFTPTLSModeRoundTrip(t *testing.T) {
	for _, mode := range []string{"none", "explicit", "implicit"} {
		if got := ftpTLSMode(ftpSecure(mode)); got != mode {
			t.Errorf("%s -> %v -> %s", mode, ftpSecure(mode), got)
		}
	}
	if got := ftpTLSMode("explicit"); got != "none" { // the worker treats the string "explicit" as plain FTP
		t.Errorf(`"explicit" string read as %s`, got)
	}
}

// ---- acceptance --------------------------------------------------------------------------------

// accCustomerName is UPTIMEIFY_ACC_CUSTOMER_NAME, else the first visible customer.
func accCustomerName(t *testing.T) string {
	if n := os.Getenv("UPTIMEIFY_ACC_CUSTOMER_NAME"); n != "" {
		return n
	}
	cs, err := accClient().ListCustomers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) == 0 {
		t.Fatal("no customer visible to the token; set UPTIMEIFY_ACC_CUSTOMER_NAME")
	}
	return cs[0].Name
}

func monitorConfig(customer, typ, body string) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    uptimeify = { source = "uptimeify/uptimeify" }
  }
}

data "uptimeify_customer" "c" { name = %q }

resource "uptimeify_%s_monitor" "m" {
  customer_id = data.uptimeify_customer.c.id
%s
}
`, customer, typ, body)
}

// testAccMonitor runs create, update in place (+ data source by id), import, drift and destroy.
func testAccMonitor(t *testing.T, typ, kind, create, update string, createCheck, updateCheck tfresource.TestCheckFunc) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	customer := accCustomerName(t)
	addr := "uptimeify_" + typ + "_monitor.m"
	var id string
	saveID := func(s *terraform.State) error {
		id = s.RootModule().Resources[addr].Primary.ID
		return nil
	}
	sameID := func(s *terraform.State) error {
		if got := s.RootModule().Resources[addr].Primary.ID; got != id {
			return fmt.Errorf("monitor was replaced: %s -> %s", id, got)
		}
		return nil
	}
	cfgUpdate := monitorConfig(customer, typ, update)

	tfresource.Test(t, tfresource.TestCase{
		PreCheck: func() {
			if os.Getenv("UPTIMEIFY_API_TOKEN") == "" {
				t.Fatal("UPTIMEIFY_API_TOKEN must be set for acceptance tests")
			}
		},
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			if _, err := accClient().GetMonitor(context.Background(), kind, id); !client.IsNotFound(err) {
				return fmt.Errorf("%s %s still exists (err=%v)", kind, id, err)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: monitorConfig(customer, typ, create),
				Check: tfresource.ComposeAggregateTestCheckFunc(createCheck, saveID,
					tfresource.TestCheckResourceAttrPair(addr, "customer_id", "data.uptimeify_customer.c", "id"),
					tfresource.TestCheckResourceAttr(addr, "status", "active"),
					tfresource.TestCheckResourceAttr(addr, "check_interval", "30"),
					tfresource.TestCheckResourceAttrSet(addr, "public_id"),
				),
			},
			{
				Config: cfgUpdate + fmt.Sprintf("\ndata \"uptimeify_%s_monitor\" \"by_id\" { id = %s.public_id }\n", typ, addr),
				Check: tfresource.ComposeAggregateTestCheckFunc(updateCheck, sameID,
					tfresource.TestCheckResourceAttrPair("data.uptimeify_"+typ+"_monitor.by_id", "name", addr, "name"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_"+typ+"_monitor.by_id", "hostname", addr, "hostname"),
				),
			},
			{ResourceName: addr, ImportState: true, ImportStateVerify: true},
			{ // drift: deleted outside Terraform -> plan not empty
				PreConfig: func() {
					if err := accClient().DeleteMonitor(context.Background(), kind, id); err != nil {
						t.Fatal(err)
					}
				},
				Config:             cfgUpdate,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: cfgUpdate, Check: saveID}, // recreate; the framework then destroys, CheckDestroy verifies
		},
	})
}

func accName(typ string) string { return fmt.Sprintf("tfacc-%s-%d", typ, time.Now().Unix()%1_000_000) }

// DNS monitors have no SSRF guard on create; example.com is fine without internet.
func TestAccDNSMonitor(t *testing.T) {
	name := accName("dns")
	a := "uptimeify_dns_monitor.m"
	testAccMonitor(t, "dns", "dns-monitors",
		fmt.Sprintf(`
  name            = %q
  hostname        = "example.com"
  record_types    = ["A"]
  match_mode      = "contains"
  expected_values = { A = ["93.184.215.14"] }
`, name),
		fmt.Sprintf(`
  name                        = "%s-2"
  hostname                    = "www.example.com"
  status                      = "disabled"
  check_interval              = 60
  timeout_seconds             = 10
  allowed_check_country_codes = ["DE"]
  record_types                = ["A", "TXT"]
  match_mode                  = "exact"
  expected_values             = { A = ["93.184.215.14"], TXT = ["v=spf1 -all"] }
  trigger_on_mismatch         = false
  require_expected_values     = true
`, name),
		tfresource.ComposeAggregateTestCheckFunc(
			tfresource.TestCheckResourceAttr(a, "name", name),
			tfresource.TestCheckResourceAttr(a, "record_types.#", "1"),
			tfresource.TestCheckResourceAttr(a, "trigger_on_mismatch", "true"),
			tfresource.TestCheckNoResourceAttr(a, "allowed_check_country_codes"),
		),
		tfresource.ComposeAggregateTestCheckFunc(
			tfresource.TestCheckResourceAttr(a, "status", "disabled"),
			tfresource.TestCheckResourceAttr(a, "check_interval", "60"),
			tfresource.TestCheckResourceAttr(a, "allowed_check_country_codes.0", "DE"),
			tfresource.TestCheckResourceAttr(a, "expected_values.TXT.0", "v=spf1 -all"),
			tfresource.TestCheckResourceAttr(a, "trigger_on_mismatch", "false"),
			tfresource.TestCheckResourceAttr("data.uptimeify_dns_monitor.by_id", "match_mode", "exact"),
		),
	)
}

// TCP and ICMP check the target on create (validatePublicHostname: DNS lookup unless it is an IP);
// the sandbox has no internet, so the targets are public IP literals.
func TestAccTCPMonitor(t *testing.T) {
	name := accName("tcp")
	a := "uptimeify_tcp_monitor.m"
	testAccMonitor(t, "tcp", "tcp-monitors",
		fmt.Sprintf(`
  name     = %q
  hostname = "1.1.1.1"
  port     = 443
`, name),
		fmt.Sprintf(`
  name          = "%s-2"
  hostname      = "8.8.8.8"
  port          = 53
  expect_banner = "hello"
  status        = "disabled"
`, name),
		tfresource.ComposeAggregateTestCheckFunc(
			tfresource.TestCheckResourceAttr(a, "port", "443"),
			tfresource.TestCheckNoResourceAttr(a, "expect_banner"),
		),
		tfresource.ComposeAggregateTestCheckFunc(
			tfresource.TestCheckResourceAttr(a, "hostname", "8.8.8.8"),
			tfresource.TestCheckResourceAttr(a, "port", "53"),
			tfresource.TestCheckResourceAttr(a, "expect_banner", "hello"),
			tfresource.TestCheckResourceAttr("data.uptimeify_tcp_monitor.by_id", "expect_banner", "hello"),
		),
	)
}

func TestAccICMPMonitor(t *testing.T) {
	name := accName("icmp")
	a := "uptimeify_icmp_monitor.m"
	testAccMonitor(t, "icmp", "icmp-monitors",
		fmt.Sprintf(`
  name     = %q
  hostname = "1.1.1.1"
`, name),
		fmt.Sprintf(`
  name        = "%s-2"
  hostname    = "1.0.0.1"
  packet_size = 64
  ping_count  = 3
  status      = "maintenance"
`, name),
		tfresource.ComposeAggregateTestCheckFunc(
			tfresource.TestCheckNoResourceAttr(a, "packet_size"),
			tfresource.TestCheckNoResourceAttr(a, "ping_count"),
		),
		tfresource.ComposeAggregateTestCheckFunc(
			tfresource.TestCheckResourceAttr(a, "packet_size", "64"),
			tfresource.TestCheckResourceAttr(a, "ping_count", "3"),
			tfresource.TestCheckResourceAttr(a, "status", "maintenance"),
		),
	)
}
