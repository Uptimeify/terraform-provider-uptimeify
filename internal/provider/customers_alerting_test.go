package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

// Unit: redacted secrets and has-flags leave config, the flags land in secrets_set, nulls are dropped,
// and an equal config keeps the configured spelling.
func TestNotificationChannelFromAPIStripsSecrets(t *testing.T) {
	m := notificationChannelModel{Config: types.StringValue(`{ "url": "https://example.com/h" }`)}
	ch := &client.NotificationChannel{ID: 3, Type: "webhook", Category: "direct", Name: "n",
		Config: map[string]any{"url": "https://example.com/h", "secret": nil, "hasSecret": true, "old": nil}}
	if d := m.fromAPI(context.Background(), ch); d.HasError() {
		t.Fatal(d)
	}
	if m.Config.ValueString() != `{ "url": "https://example.com/h" }` {
		t.Fatalf("config = %s", m.Config.ValueString())
	}
	if v, ok := m.SecretsSet.Elements()["Secret"]; !ok || v.String() != "true" {
		t.Fatalf("secrets_set = %v", m.SecretsSet)
	}
	if !m.Conditions.IsNull() || !m.AllowedPackageTypes.IsNull() || m.DelaySeconds.ValueInt64() != 0 {
		t.Fatal("unset fields must be null / 0")
	}
}

// Unit: an update that drops a config key sends it as null (the API merges onto the stored config)
// and resends the write-only secrets only when the version changes.
func TestNotificationChannelUpdateClearsRemovedKeys(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_, _ = w.Write([]byte(`{"id":3,"organizationId":1,"type":"webhook","category":"direct","name":"n","config":{"url":"https://example.com/h","hasSecret":true},"delaySeconds":0,"isActive":true,"createdAt":"x"}`))
	}))
	defer srv.Close()
	ctx := context.Background()
	r := &notificationChannelResource{client: client.New(srv.URL, "wsm_test", "test")}
	var sch resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sch)

	base := notificationChannelResourceModel{notificationChannelModel: notificationChannelModel{
		ID: types.StringValue("3"), OrganizationID: types.Int64Value(1), CustomerID: types.Int64Null(),
		Type: types.StringValue("webhook"), Category: types.StringValue("direct"), Name: types.StringValue("n"),
		SecretsSet: types.MapNull(types.BoolType), DelaySeconds: types.Int64Value(0), Conditions: types.StringNull(),
		AllowedPackageTypes: types.SetNull(types.StringType), IsActive: types.BoolValue(true), CreatedAt: types.StringValue("x"),
	}, SecretConfigWO: types.StringNull(), SecretConfigWOVersion: types.Int64Value(1)}
	state, plan, cfg := base, base, base
	state.Config = types.StringValue(`{"url":"https://example.com/h","method":"POST"}`)
	plan.Config = types.StringValue(`{"url":"https://example.com/h"}`)
	cfg.Config = plan.Config
	cfg.SecretConfigWO = types.StringValue(`{"secret":"s3"}`) // same version: not resent

	set := func(v notificationChannelResourceModel) tfsdk.Plan {
		p := tfsdk.Plan{Schema: sch.Schema}
		if d := p.Set(ctx, &v); d.HasError() {
			t.Fatal(d)
		}
		return p
	}
	st := tfsdk.State{Schema: sch.Schema}
	if d := st.Set(ctx, &state); d.HasError() {
		t.Fatal(d)
	}
	p := set(plan)
	c := tfsdk.Config{Schema: sch.Schema, Raw: set(cfg).Raw}
	resp := resource.UpdateResponse{State: st}
	r.Update(ctx, resource.UpdateRequest{Plan: p, State: st, Config: c}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	conf, _ := got["config"].(map[string]any)
	if v, ok := conf["method"]; !ok || v != nil {
		t.Fatalf("removed key not cleared: %v", got)
	}
	if _, ok := conf["secret"]; ok {
		t.Fatalf("secret resent without version change: %v", got)
	}
}

// Acceptance: customer, IP and domain on a customer the test creates itself. The domain uses a
// .com name (offline fallback TLD list) and 192.0.2.0/24 (TEST-NET-1); POST checks neither live.
func TestAccCustomerIPDomain(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	accPreCheck(t)
	n := time.Now().Unix() % 1_000_000
	name := fmt.Sprintf("tfacc-cust-%d", n)
	pkg := accPackageType(t)
	ip := fmt.Sprintf("192.0.2.%d", n%250+1)
	domain := fmt.Sprintf("tfacc-%d.example.com", n)
	var custID, ipID, domID string

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			ctx := context.Background()
			if _, err := accClient().GetCustomerIP(ctx, ipID); !client.IsNotFound(err) {
				return fmt.Errorf("ip %s still exists (err=%v)", ipID, err)
			}
			if _, err := accClient().GetCustomerDomain(ctx, domID); !client.IsNotFound(err) {
				return fmt.Errorf("domain %s still exists (err=%v)", domID, err)
			}
			if _, err := accClient().GetCustomer(ctx, custID); !client.IsNotFound(err) {
				return fmt.Errorf("customer %s still exists (err=%v)", custID, err)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: customerConfig(name, pkg, `custom_fields = jsonencode({})`, ip, "", domain, "active"),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_customer.c", "name", name),
					tfresource.TestCheckResourceAttr("uptimeify_customer.c", "status", "active"),
					tfresource.TestCheckResourceAttrSet("uptimeify_customer.c", "package_id"),
					tfresource.TestCheckResourceAttr("uptimeify_customer.c", "notification_targets.email", "customer"),
					tfresource.TestCheckResourceAttr("uptimeify_customer_ip.i", "ip_family", "v4"),
					tfresource.TestCheckResourceAttr("uptimeify_customer_ip.i", "management_type", "managed"),
					tfresource.TestCheckResourceAttr("uptimeify_customer_domain.d", "expiry_warning_days", "30"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources
						custID, ipID, domID = rs["uptimeify_customer.c"].Primary.ID, rs["uptimeify_customer_ip.i"].Primary.ID, rs["uptimeify_customer_domain.d"].Primary.ID
						return nil
					},
				),
			},
			{ // update in place
				Config: customerConfig(name+"-2", pkg, `company_name = "tfacc GmbH"
  notification_email = "tfacc@example.com"
  can_edit_managed = true
  custom_fields = jsonencode({})`, ip, `label = "tfacc"
  status = "disabled"`, domain, "disabled"),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_customer.c", "name", name+"-2"),
					tfresource.TestCheckResourceAttr("uptimeify_customer.c", "company_name", "tfacc GmbH"),
					tfresource.TestCheckResourceAttr("uptimeify_customer_ip.i", "label", "tfacc"),
					tfresource.TestCheckResourceAttr("uptimeify_customer_ip.i", "status", "disabled"),
					tfresource.TestCheckResourceAttr("uptimeify_customer_domain.d", "status", "disabled"),
					tfresource.TestCheckResourceAttr("uptimeify_customer_domain.d", "expiry_error_days", "5"),
					func(s *terraform.State) error {
						if got := s.RootModule().Resources["uptimeify_customer.c"].Primary.ID; got != custID {
							return fmt.Errorf("customer replaced: %s -> %s", custID, got)
						}
						return nil
					},
				),
			},
			{ResourceName: "uptimeify_customer.c", ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: append([]string{"package_type", "delete_user_action"}, customerOverrides...)},
			{ResourceName: "uptimeify_customer_ip.i", ImportState: true, ImportStateVerify: true},
			{ResourceName: "uptimeify_customer_domain.d", ImportState: true, ImportStateVerify: true},
			{ // drift: IP and domain deleted outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteCustomerIP(context.Background(), ipID); err != nil {
						t.Fatal(err)
					}
					if err := accClient().DeleteCustomerDomain(context.Background(), domID); err != nil {
						t.Fatal(err)
					}
				},
				Config: customerConfig(name+"-2", pkg, `company_name = "tfacc GmbH"
  notification_email = "tfacc@example.com"
  can_edit_managed = true
  custom_fields = jsonencode({})`, ip, `label = "tfacc"
  status = "disabled"`, domain, "disabled"),
				PlanOnly: true, ExpectNonEmptyPlan: true,
			},
			{ // recreate, then destroy and CheckDestroy
				Config: customerConfig(name+"-2", pkg, `custom_fields = jsonencode({})`, ip, `status = "disabled"`, domain, "disabled"),
				Check: func(s *terraform.State) error {
					rs := s.RootModule().Resources
					ipID, domID = rs["uptimeify_customer_ip.i"].Primary.ID, rs["uptimeify_customer_domain.d"].Primary.ID
					return nil
				},
			},
		},
	})
}

// Acceptance: an email channel (no secrets) and a webhook channel with a write-only HMAC secret.
func TestAccNotificationChannel(t *testing.T) {
	n := time.Now().Unix() % 1_000_000
	name := fmt.Sprintf("tfacc-ch-%d", n)
	var mailID, hookID string
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			for _, id := range []string{mailID, hookID} {
				if _, err := accClient().GetNotificationChannel(context.Background(), id); !client.IsNotFound(err) {
					return fmt.Errorf("channel %s still exists (err=%v)", id, err)
				}
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: channelConfig(name, n, 0, 1),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_notification_channel.mail", "category", "direct"),
					tfresource.TestCheckResourceAttr("uptimeify_notification_channel.mail", "is_active", "true"),
					tfresource.TestCheckResourceAttr("uptimeify_notification_channel.hook", "secrets_set.Secret", "true"),
					tfresource.TestCheckNoResourceAttr("uptimeify_notification_channel.hook", "secret_config_wo"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources
						mailID, hookID = rs["uptimeify_notification_channel.mail"].Primary.ID, rs["uptimeify_notification_channel.hook"].Primary.ID
						return nil
					},
				),
			},
			{ // update: delay, name, config key removed, secret resent; data sources
				Config: channelConfig(name+"-2", n, 60, 2) + `
data "uptimeify_notification_channel" "by_id"   { id = uptimeify_notification_channel.hook.id }
data "uptimeify_notification_channel" "by_name" { name = uptimeify_notification_channel.mail.name }
`,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_notification_channel.mail", "delay_seconds", "60"),
					tfresource.TestCheckResourceAttr("uptimeify_notification_channel.mail", "name", name+"-2-mail"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_notification_channel.by_id", "config", "uptimeify_notification_channel.hook", "config"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_notification_channel.by_name", "id", "uptimeify_notification_channel.mail", "id"),
					func(s *terraform.State) error {
						if got := s.RootModule().Resources["uptimeify_notification_channel.hook"].Primary.ID; got != hookID {
							return fmt.Errorf("channel replaced: %s -> %s", hookID, got)
						}
						return nil
					},
				),
			},
			{ResourceName: "uptimeify_notification_channel.mail", ImportState: true, ImportStateVerify: true},
			{ResourceName: "uptimeify_notification_channel.hook", ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"secret_config_wo_version"}},
			{
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteNotificationChannel(context.Background(), mailID); err != nil {
						t.Fatal(err)
					}
				},
				Config:   channelConfig(name+"-2", n, 60, 2),
				PlanOnly: true, ExpectNonEmptyPlan: true,
			},
			{
				Config: channelConfig(name+"-2", n, 60, 2),
				Check: func(s *terraform.State) error {
					mailID = s.RootModule().Resources["uptimeify_notification_channel.mail"].Primary.ID
					return nil
				},
			},
		},
	})
}

// Acceptance: the organization's escalation config without a webhook URL (the API resolves the URL
// host, which the offline sandbox cannot). Mutates the organization singleton.
func TestAccEscalationConfig(t *testing.T) {
	var orgID int64
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			e, err := accClient().GetEscalationConfig(context.Background(), orgID)
			if err != nil {
				return err
			}
			if e.IsActive || e.WebhookURL != nil && *e.WebhookURL != "" {
				return fmt.Errorf("escalation config still active after destroy: %+v", e)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: escalationConfig(10, "200,204"),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_escalation_config.e", "webhook_method", "PUT"),
					tfresource.TestCheckResourceAttr("uptimeify_escalation_config.e", "webhook_timeout", "10"),
					tfresource.TestCheckResourceAttrSet("uptimeify_escalation_config.e", "webhook_retry_delay"),
					func(s *terraform.State) error {
						_, err := fmt.Sscan(s.RootModule().Resources["uptimeify_escalation_config.e"].Primary.ID, &orgID)
						if err != nil {
							return err
						}
						e, err := accClient().GetEscalationConfig(context.Background(), orgID)
						if err == nil && e.WebhookHeaders["X-Tfacc"] != "1" {
							err = fmt.Errorf("write-only headers not stored: %v", e.WebhookHeaders)
						}
						return err
					},
				),
			},
			{
				Config: escalationConfig(15, "200"),
				Check:  tfresource.TestCheckResourceAttr("uptimeify_escalation_config.e", "webhook_timeout", "15"),
			},
			{ResourceName: "uptimeify_escalation_config.e", ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"webhook_headers_wo_version"}},
			{ // drift: changed outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if _, err := accClient().UpdateEscalationConfig(context.Background(), orgID, map[string]any{"webhookTimeout": 20}); err != nil {
						t.Fatal(err)
					}
				},
				Config:   escalationConfig(15, "200"),
				PlanOnly: true, ExpectNonEmptyPlan: true,
			},
			{Config: escalationConfig(15, "200")},
		},
	})
}

// Acceptance: destroy deactivates, re-create with the same key reactivates the same id.
func TestAccCustomField(t *testing.T) {
	key := fmt.Sprintf("tfacc_%d", time.Now().Unix()%1_000_000)
	var fieldID string
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			f, err := accClient().GetCustomField(context.Background(), fieldID)
			if client.IsNotFound(err) || err == nil && !f.IsActive {
				return nil
			}
			return fmt.Errorf("custom field %s still active (err=%v)", fieldID, err)
		},
		Steps: []tfresource.TestStep{
			{
				Config: customFieldConfig(key, "text", ""),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_custom_field.f", "field_key", key),
					tfresource.TestCheckResourceAttr("uptimeify_custom_field.f", "show_in_table", "true"),
					tfresource.TestCheckResourceAttr("uptimeify_custom_field.f", "options.#", "0"),
					func(s *terraform.State) error {
						fieldID = s.RootModule().Resources["uptimeify_custom_field.f"].Primary.ID
						return nil
					},
				),
			},
			{
				Config: customFieldConfig(key, "select", `options = ["a", "b"]
  placeholder = "pick"`) + `
data "uptimeify_custom_field" "k" { field_key = uptimeify_custom_field.f.field_key }
`,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_custom_field.f", "field_type", "select"),
					tfresource.TestCheckResourceAttr("uptimeify_custom_field.f", "options.1", "b"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_custom_field.k", "id", "uptimeify_custom_field.f", "id"),
				),
			},
			{ResourceName: "uptimeify_custom_field.f", ImportState: true, ImportStateVerify: true},
			{ // drift: deactivated outside Terraform counts as gone
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteCustomField(context.Background(), fieldID); err != nil {
						t.Fatal(err)
					}
				},
				Config:   customFieldConfig(key, "select", `options = ["a", "b"]`),
				PlanOnly: true, ExpectNonEmptyPlan: true,
			},
			{ // create with the same key reactivates the deactivated field
				Config: customFieldConfig(key, "select", `options = ["a", "b"]`),
				Check: func(s *terraform.State) error {
					if got := s.RootModule().Resources["uptimeify_custom_field.f"].Primary.ID; got != fieldID {
						return fmt.Errorf("expected reactivation of %s, got new id %s", fieldID, got)
					}
					return nil
				},
			},
		},
	})
}

// accPackageType is the first package of the organization (GET /api/package-configs).
func accPackageType(t *testing.T) string {
	t.Helper()
	accPreCheck(t)
	var pkgs []struct {
		PackageType string `json:"packageType"`
	}
	if err := accClient().Do(context.Background(), http.MethodGet, "/api/package-configs", nil, &pkgs); err != nil || len(pkgs) == 0 {
		t.Fatalf("no package configuration for the acceptance customer (err=%v)", err)
	}
	return pkgs[0].PackageType
}

func customerConfig(name, pkg, extra, ip, ipExtra, domain, domainStatus string) string {
	errDays := "7"
	if domainStatus == "disabled" {
		errDays = "5"
	}
	return accProviderBlock + fmt.Sprintf(`
resource "uptimeify_customer" "c" {
  name         = %q
  package_type = %q
  notification_targets = { email = "customer" }
  %s
}

resource "uptimeify_customer_ip" "i" {
  customer_id = tonumber(uptimeify_customer.c.id)
  ip_address  = %q
  %s
}

resource "uptimeify_customer_domain" "d" {
  customer_id       = tonumber(uptimeify_customer.c.id)
  domain_name       = %q
  status            = %q
  expiry_error_days = %s
}
`, name, pkg, extra, ip, ipExtra, domain, domainStatus, errDays)
}

func channelConfig(name string, n int64, delay, secretVersion int) string {
	hookCfg := `{ url = "https://example.com/tfacc-hook", method = "POST" }`
	if secretVersion > 1 {
		hookCfg = `{ url = "https://example.com/tfacc-hook" }`
	}
	return accProviderBlock + fmt.Sprintf(`
resource "uptimeify_notification_channel" "mail" {
  type          = "email"
  name          = "%s-mail"
  config        = jsonencode({ email = "tfacc-%d@example.com" })
  delay_seconds = %d
}

resource "uptimeify_notification_channel" "hook" {
  type                     = "webhook"
  name                     = "%s-hook"
  config                   = jsonencode(%s)
  secret_config_wo         = jsonencode({ secret = "tfacc-secret-%d" })
  secret_config_wo_version = %d
}
`, name, n, delay, name, hookCfg, secretVersion, secretVersion)
}

func escalationConfig(timeout int, codes string) string {
	return accProviderBlock + fmt.Sprintf(`
resource "uptimeify_escalation_config" "e" {
  webhook_method             = "PUT"
  webhook_timeout            = %d
  webhook_retry_attempts     = 2
  expected_status_codes      = %q
  is_active                  = true
  webhook_headers_wo         = { "X-Tfacc" = "1" }
  webhook_headers_wo_version = 1
}
`, timeout, codes)
}

func customFieldConfig(key, typ, extra string) string {
	return accProviderBlock + fmt.Sprintf(`
resource "uptimeify_custom_field" "f" {
  name       = "%s name"
  field_key  = %q
  field_type = %q
  %s
}
`, strings.ReplaceAll(key, "_", " "), key, typ, extra)
}
