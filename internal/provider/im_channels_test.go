package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Unit: the channel PATCH carries only changed fields; secrets only on a version bump (headers then
// replace the stored set, unset = {}), the URL also on a type change; a removed email is null.
func TestImChannelPatch(t *testing.T) {
	ctx := context.Background()
	base := func() imChannelResourceModel {
		var m imChannelResourceModel
		m.Name, m.Type = types.StringValue("n"), types.StringValue("slack")
		m.TeamID, m.Email = types.Int64Null(), types.StringNull()
		m.IsActive, m.SecretsWOVersion = types.BoolValue(true), types.Int64Value(1)
		m.WebhookURLWO, m.HeadersWO = types.StringNull(), types.MapNull(types.StringType)
		return m
	}
	state, plan, cfg := base(), base(), base()
	cfg.WebhookURLWO = types.StringValue("https://hooks.example.com/x")
	if got := mustJSON(t, channelPatch(ctx, &plan, &state, &cfg)); got != `{}` {
		t.Errorf("no change: %s", got)
	}
	plan.SecretsWOVersion = types.Int64Value(2)
	if got := mustJSON(t, channelPatch(ctx, &plan, &state, &cfg)); got != `{"config":{"headers":{},"webhookUrl":"https://hooks.example.com/x"}}` {
		t.Errorf("version bump: %s", got)
	}
	plan = base()
	plan.Type, plan.Email, plan.TeamID = types.StringValue("email"), types.StringValue("a@example.com"), types.Int64Value(3)
	state.Email = types.StringValue("old@example.com")
	if got := mustJSON(t, channelPatch(ctx, &plan, &state, &cfg)); got != `{"config":{"email":"a@example.com","webhookUrl":"https://hooks.example.com/x"},"teamId":3,"type":"email"}` {
		t.Errorf("type change: %s", got)
	}
	plan, cfg = base(), base()
	if got := mustJSON(t, channelPatch(ctx, &plan, &state, &cfg)); got != `{"config":{"email":null}}` {
		t.Errorf("email removed: %s", got)
	}
}

// Unit: the outbound PATCH replaces the whole filters object and keeps the stored event kinds when
// the plan leaves them unknown; secrets go out only on a version bump.
func TestImOutboundPatch(t *testing.T) {
	ctx := context.Background()
	base := func() imOutboundResourceModel {
		var m imOutboundResourceModel
		m.Name, m.Type = types.StringValue("n"), types.StringValue("webhook")
		m.TeamID, m.IsActive = types.Int64Null(), types.BoolValue(true)
		m.FilterTeamIDs, m.FilterSeverity = types.ListNull(types.Int64Type), types.ListNull(types.StringType)
		m.EventKinds = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("resolved")})
		m.Email, m.BaseURL, m.ProjectKey = types.StringNull(), types.StringNull(), types.StringNull()
		m.SecretsWOVersion = types.Int64Value(1)
		m.WebhookURLWO, m.BearerTokenWO, m.APITokenWO = types.StringNull(), types.StringNull(), types.StringNull()
		m.RoutingKeyWO, m.APIKeyWO, m.HeadersWO = types.StringNull(), types.StringNull(), types.MapNull(types.StringType)
		return m
	}
	state, plan, cfg := base(), base(), base()
	cfg.BearerTokenWO = types.StringValue("tok")
	plan.EventKinds = types.ListUnknown(types.StringType)
	if got := mustJSON(t, outboundPatch(ctx, &plan, &state, &cfg)); got != `{}` {
		t.Errorf("unknown event kinds alone: %s", got)
	}
	plan.FilterSeverity = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("sev1")})
	if got := mustJSON(t, outboundPatch(ctx, &plan, &state, &cfg)); got != `{"filters":{"eventKinds":["resolved"],"severities":["sev1"]}}` {
		t.Errorf("filters: %s", got)
	}
	plan = base()
	plan.SecretsWOVersion = types.Int64Value(2)
	if got := mustJSON(t, outboundPatch(ctx, &plan, &state, &cfg)); got != `{"config":{"bearerToken":"tok","headers":{}}}` {
		t.Errorf("version bump: %s", got)
	}
}

// Acceptance: org-wide Slack channel with write-only URL and headers, team-scoped email channel.
func imChannelConfig(team int64, name string, step int) string {
	active, hdr, ver, email := "true", `{ "X-Tfacc" = "a" }`, 1, "tfacc@example.com"
	if step == 2 {
		active, hdr, ver, email = "false", `{ "X-Tfacc" = "b", "X-Second" = "c" }`, 2, "tfacc2@example.com"
	}
	return imProviderBlock() + fmt.Sprintf(`
resource "uptimeify_im_channel" "slack" {
  name               = "%[2]s-slack"
  type               = "slack"
  is_active          = %[3]s
  webhook_url_wo     = "https://hooks.slack.com/services/T000/B000/tfacc%[5]d"
  headers_wo         = %[4]s
  secrets_wo_version = %[5]d
}

resource "uptimeify_im_channel" "mail" {
  name    = "%[2]s-mail"
  type    = "email"
  team_id = %[1]d
  email   = %[6]q
}

data "uptimeify_im_channel" "by_name" { name = uptimeify_im_channel.slack.name }
`, team, name, active, hdr, ver, email)
}

func TestAccImChannel(t *testing.T) {
	team := accImTeam(t)
	name := fmt.Sprintf("tfacc-ch-%d", time.Now().Unix()%1_000_000)
	var slackID, mailID string
	const s, m = "uptimeify_im_channel.slack", "uptimeify_im_channel.mail"

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			for _, id := range []string{slackID, mailID} {
				if _, err := accClient().GetImChannel(context.Background(), id); !client.IsNotFound(err) {
					return fmt.Errorf("channel %s still exists (err=%v)", id, err)
				}
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: imChannelConfig(team, name, 1),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(s, "has_webhook_url", "true"),
					tfresource.TestCheckResourceAttr(s, "header_names.#", "1"),
					tfresource.TestCheckResourceAttr(s, "header_names.0", "X-Tfacc"),
					tfresource.TestCheckResourceAttr(s, "is_active", "true"),
					tfresource.TestCheckNoResourceAttr(s, "team_id"),
					tfresource.TestCheckNoResourceAttr(s, "webhook_url_wo"),
					tfresource.TestCheckNoResourceAttr(s, "headers_wo"),
					tfresource.TestCheckResourceAttr(m, "team_id", fmt.Sprint(team)),
					tfresource.TestCheckResourceAttr(m, "email", "tfacc@example.com"),
					tfresource.TestCheckResourceAttr(m, "has_webhook_url", "false"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_im_channel.by_name", "id", s, "id"),
					func(st *terraform.State) error {
						slackID = st.RootModule().Resources[s].Primary.ID
						mailID = st.RootModule().Resources[m].Primary.ID
						return nil
					},
				),
			},
			{ // update in place: deactivate, new headers (version bump), new email
				Config: imChannelConfig(team, name, 2),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(s, "is_active", "false"),
					tfresource.TestCheckResourceAttr(s, "header_names.#", "2"),
					tfresource.TestCheckResourceAttr(s, "header_names.0", "X-Second"),
					tfresource.TestCheckResourceAttr(m, "email", "tfacc2@example.com"),
					tfresource.TestCheckResourceAttr("data.uptimeify_im_channel.by_name", "is_active", "false"),
					func(st *terraform.State) error {
						if st.RootModule().Resources[s].Primary.ID != slackID {
							return fmt.Errorf("slack channel was replaced")
						}
						return nil
					},
				),
			},
			{
				ResourceName:            s,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"secrets_wo_version"},
			},
			{
				ResourceName:      m,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // drift: deleted outside Terraform -> plan not empty
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteImChannel(context.Background(), slackID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             withoutDataSources(imChannelConfig(team, name, 2)),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // recreate, then destroy + CheckDestroy
				Config: imChannelConfig(team, name, 2),
				Check: func(st *terraform.State) error {
					slackID = st.RootModule().Resources[s].Primary.ID
					return nil
				},
			},
		},
	})
}

// Acceptance: one integration per secret shape. The targets are never called on save.
func imOutboundConfig(team int64, name string, step int) string {
	kinds, sev, active, ver, hook := `["incident_created", "resolved"]`, `["sev1"]`, "true", 1, "a"
	if step == 2 {
		kinds, sev, active, ver, hook = `["resolved"]`, `["sev1", "sev2"]`, "false", 2, "b"
	}
	return imProviderBlock() + fmt.Sprintf(`
resource "uptimeify_im_outbound_integration" "hook" {
  name               = "%[2]s-hook"
  type               = "webhook"
  team_id            = %[1]d
  is_active          = %[5]s
  filter_team_ids    = [%[1]d]
  filter_severities  = %[4]s
  event_kinds        = %[3]s
  webhook_url_wo     = "https://hooks.example.com/tfacc-%[7]s"
  bearer_token_wo    = "tfacc-bearer-%[7]s"
  headers_wo         = { "X-Tfacc" = "%[7]s" }
  secrets_wo_version = %[6]d
}

resource "uptimeify_im_outbound_integration" "jira" {
  name         = "%[2]s-jira"
  type         = "jira"
  email        = "tfacc@example.com"
  base_url     = "https://tfacc.atlassian.net"
  project_key  = "TFACC"
  api_token_wo = "tfacc-jira-token"
}

resource "uptimeify_im_outbound_integration" "pd" {
  name           = "%[2]s-pd"
  type           = "pagerduty_v2"
  routing_key_wo = "tfacc-routing-key"
}

data "uptimeify_im_outbound_integration" "by_name" { name = uptimeify_im_outbound_integration.hook.name }
`, team, name, kinds, sev, active, ver, hook)
}

func TestAccImOutboundIntegration(t *testing.T) {
	team := accImTeam(t)
	name := fmt.Sprintf("tfacc-ob-%d", time.Now().Unix()%1_000_000)
	ids := map[string]string{}
	const h, j, p = "uptimeify_im_outbound_integration.hook", "uptimeify_im_outbound_integration.jira", "uptimeify_im_outbound_integration.pd"
	capture := func(st *terraform.State) error {
		for _, r := range []string{h, j, p} {
			ids[r] = st.RootModule().Resources[r].Primary.ID
		}
		return nil
	}

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			for _, id := range ids {
				if _, err := accClient().GetImOutbound(context.Background(), id); !client.IsNotFound(err) {
					return fmt.Errorf("outbound integration %s still exists (err=%v)", id, err)
				}
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: imOutboundConfig(team, name, 1),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(h, "has_webhook_url", "true"),
					tfresource.TestCheckResourceAttr(h, "has_bearer_token", "true"),
					tfresource.TestCheckResourceAttr(h, "has_headers", "true"),
					tfresource.TestCheckResourceAttr(h, "event_kinds.#", "2"),
					tfresource.TestCheckResourceAttr(h, "filter_team_ids.0", fmt.Sprint(team)),
					tfresource.TestCheckNoResourceAttr(h, "bearer_token_wo"),
					tfresource.TestCheckResourceAttr(j, "event_kinds.#", "1"), // server default for jira
					tfresource.TestCheckResourceAttr(j, "event_kinds.0", "incident_created"),
					tfresource.TestCheckResourceAttr(j, "has_api_token", "true"),
					tfresource.TestCheckResourceAttr(j, "base_url", "https://tfacc.atlassian.net"),
					tfresource.TestCheckNoResourceAttr(j, "team_id"),
					tfresource.TestCheckResourceAttr(p, "has_routing_key", "true"),
					tfresource.TestCheckNoResourceAttr(p, "event_kinds"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_im_outbound_integration.by_name", "id", h, "id"),
					capture,
				),
			},
			{ // update in place: filters, deactivate, secrets resent
				Config: imOutboundConfig(team, name, 2),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(h, "is_active", "false"),
					tfresource.TestCheckResourceAttr(h, "event_kinds.#", "1"),
					tfresource.TestCheckResourceAttr(h, "filter_severities.#", "2"),
					tfresource.TestCheckResourceAttr(h, "has_headers", "true"),
					tfresource.TestCheckResourceAttr("data.uptimeify_im_outbound_integration.by_name", "is_active", "false"),
					func(st *terraform.State) error {
						if st.RootModule().Resources[h].Primary.ID != ids[h] {
							return fmt.Errorf("webhook integration was replaced")
						}
						return nil
					},
				),
			},
			{ResourceName: h, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"secrets_wo_version"}},
			{ResourceName: j, ImportState: true, ImportStateVerify: true},
			{ // drift: deleted outside Terraform -> plan not empty
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteImOutbound(context.Background(), ids[h]); err != nil {
						t.Fatal(err)
					}
				},
				Config:             withoutDataSources(imOutboundConfig(team, name, 2)),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: imOutboundConfig(team, name, 2), Check: capture},
		},
	})
}

// Acceptance: the on-call read works for the organization and for a team (no tiers yet = empty).
func TestAccImOnCall(t *testing.T) {
	team := accImTeam(t)
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		Steps: []tfresource.TestStep{{
			Config: imProviderBlock() + fmt.Sprintf(`
data "uptimeify_im_on_call" "org" {}
data "uptimeify_im_on_call" "team" { team_id = %d }
`, team),
			Check: tfresource.ComposeAggregateTestCheckFunc(
				tfresource.TestCheckResourceAttrSet("data.uptimeify_im_on_call.org", "entries.#"),
				tfresource.TestCheckNoResourceAttr("data.uptimeify_im_on_call.org", "tiers"),
				tfresource.TestCheckResourceAttr("data.uptimeify_im_on_call.team", "entries.#", "0"),
				tfresource.TestCheckResourceAttr("data.uptimeify_im_on_call.team", "tiers.#", "0"),
			),
		}},
	})
}
