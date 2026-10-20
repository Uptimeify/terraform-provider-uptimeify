package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// Unit: v1 and v2 mappings survive API JSON -> model -> API JSON unchanged (the API stores them verbatim),
// including empty lists and maps.
func TestPayloadMappingRoundTrip(t *testing.T) {
	for _, in := range []string{
		`{"title":"trigger_name","severity":"event_severity","severityMap":{"Disaster":"sev1"},"resolveValues":["OK"],"custom":{"ip":"host_ip"}}`,
		`{"version":2,"attributes":[{"key":"title","standard":true,"steps":[{"kind":"extract","type":"path","expr":"a.b"},` +
			`{"kind":"valueMap","entries":[{"from":"x","to":"y"}],"fallback":"z"}],"flags":{"hidden":false}},` +
			`{"key":"empty","steps":[]}],"defaultSeverity":"sev2"}`,
		`{"title":"t","resolveValues":[],"custom":{}}`,
	} {
		var api client.PayloadMapping
		if err := json.Unmarshal([]byte(in), &api); err != nil {
			t.Fatal(err)
		}
		m := payloadMappingFromAPI(api)
		if m == nil {
			t.Fatalf("%s: read as empty", in)
		}
		out, _ := json.Marshal(m.toAPI())
		var a, b any
		_ = json.Unmarshal([]byte(in), &a)
		_ = json.Unmarshal(out, &b)
		if !jsonEqual(a, b) {
			t.Errorf("round trip changed the mapping:\n in: %s\nout: %s", in, out)
		}
	}
	if payloadMappingFromAPI(client.PayloadMapping{}) != nil {
		t.Error("empty mapping {} should read as null")
	}
}

// Unit: an empty stored match is null unless match = {} was written; set fields round-trip.
func TestRoutingMatchFromAPI(t *testing.T) {
	var m imRoutingRuleModel
	m.fromAPI(&client.RoutingRule{ID: 1, Match: &client.RoutingMatch{}})
	if m.Match != nil {
		t.Error("catch-all match should stay null")
	}
	m.Match = &routingMatchModel{}
	m.fromAPI(&client.RoutingRule{ID: 1, Match: &client.RoutingMatch{}})
	if m.Match == nil {
		t.Error("match = {} should stay an object")
	}
	var api client.RoutingMatch
	_ = json.Unmarshal([]byte(`{"source_ids":[3],"sourceIds":[3],"tags":[],"fieldMatches":{"env":"prod"}}`), &api)
	m.fromAPI(&client.RoutingRule{ID: 1, Match: &api})
	if got, _ := json.Marshal(m.Match.toAPI()); string(got) != `{"sourceIds":[3],"tags":[],"fieldMatches":{"env":"prod"}}` {
		t.Errorf("match round trip: %s", got)
	}
}

// accImTeam activates Incident Management for the token's organization and creates a test team,
// deleted again after the test. It skips without TF_ACC, like tfresource.Test.
func accImTeam(t *testing.T) int64 {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	ctx := context.Background()
	accPreCheck(t)
	if err := accClient().Do(ctx, http.MethodPost, "/api/im/activate", nil, nil); err != nil {
		t.Fatalf("activating Incident Management: %v", err)
	}
	var team struct {
		ID int64 `json:"id"`
	}
	accPreCheck(t)
	name := fmt.Sprintf("tfacc-team-%d", time.Now().UnixNano()%1_000_000)
	if err := accClient().Do(ctx, http.MethodPost, "/api/im/teams", map[string]any{"name": name}, &team); err != nil {
		t.Fatalf("creating test team: %v", err)
	}
	t.Cleanup(func() {
		accPreCheck(t)
		if err := accClient().Do(ctx, http.MethodDelete, "/api/im/teams/"+strconv.FormatInt(team.ID, 10), nil, nil); err != nil && !client.IsNotFound(err) {
			t.Errorf("deleting test team %d: %v", team.ID, err)
		}
	})
	return team.ID
}

func imProviderBlock() string {
	return `
terraform {
  required_providers {
    uptimeify = { source = "uptimeify/uptimeify" }
  }
}
`
}

func alertSourceConfig(team int64, name, step string) string {
	body := fmt.Sprintf(`
resource "uptimeify_im_alert_source" "s" {
  team_id                 = %d
  name                    = %q
  type                    = "custom"
  grouping_window_minutes = 5
  hmac_secret_wo          = "tfacc-hmac-secret-0123456789"
  hmac_secret_wo_version  = 1
  bearer_enabled          = true
  payload_mapping = {
    default_severity = "sev2"
    attributes = [
      { key = "title", standard = true, steps = [{ kind = "extract", type = "path", expr = "title" }] },
      { key = "severity", standard = true, steps = [
        { kind = "extract", type = "path", expr = "level" },
        { kind = "valueMap", entries = [{ from = "crit", to = "sev1" }], fallback = "sev3" },
      ] },
    ]
  }
  maintenance_windows = [{ days_of_week = [6, 7], from = "01:00", to = "03:00", timezone = "Europe/Berlin" }]
}

resource "uptimeify_im_alert_source" "z" {
  team_id = %d
  name    = "%s-zabbix"
  type    = "zabbix"
}
`, team, name, team, name)
	if step == "update" {
		body = fmt.Sprintf(`
resource "uptimeify_im_alert_source" "s" {
  team_id                = %d
  name                   = "%s-2"
  type                   = "custom"
  auto_resolve           = false
  hmac_secret_wo         = "tfacc-hmac-secret-0123456789"
  hmac_secret_wo_version = 1
  hmac_enabled           = false
  bearer_enabled         = true
  bearer_token_rotation  = "1"
  ingest_token_rotation  = "1"
  url_token_enabled      = true
  payload_mapping = {
    title          = "title || message"
    status         = "status"
    resolve_values = ["resolved"]
    severity_map   = { crit = "sev1" }
  }
}

resource "uptimeify_im_alert_source" "z" {
  team_id = %d
  name    = "%s-zabbix"
  type    = "zabbix"
}

data "uptimeify_im_alert_source" "by_name" { name = uptimeify_im_alert_source.s.name }
data "uptimeify_im_alert_source" "by_id"   { id = uptimeify_im_alert_source.s.id }
data "uptimeify_im_alert_source" "z"       { id = uptimeify_im_alert_source.z.id }
`, team, name, team, name)
	}
	return imProviderBlock() + body
}

// Acceptance: needs TF_ACC=1, UPTIMEIFY_ENDPOINT, UPTIMEIFY_API_TOKEN (org admin) and
// TF_ACC_PROVIDER_NAMESPACE=uptimeify (see scripts/acceptance.sh). Activates Incident Management.
func TestAccImAlertSource(t *testing.T) {
	team := accImTeam(t)
	name := fmt.Sprintf("tfacc-src-%d", time.Now().Unix()%1_000_000)
	var srcID, zID, firstToken, firstBearer string
	attr := func(s *terraform.State, res, key string) string {
		return s.RootModule().Resources[res].Primary.Attributes[key]
	}

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			for _, id := range []string{srcID, zID} {
				if _, err := accClient().GetAlertSource(context.Background(), id); !client.IsNotFound(err) {
					return fmt.Errorf("alert source %s still exists (err=%v)", id, err)
				}
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{ // create: v2 mapping, HMAC secret, bearer minted, maintenance; preset source reads the preset mapping
				Config: alertSourceConfig(team, name, "create"),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_im_alert_source.s", "name", name),
					tfresource.TestCheckResourceAttr("uptimeify_im_alert_source.s", "auto_resolve", "true"),
					tfresource.TestCheckResourceAttr("uptimeify_im_alert_source.s", "hmac_enabled", "true"),
					tfresource.TestCheckResourceAttr("uptimeify_im_alert_source.s", "bearer_enabled", "true"),
					tfresource.TestCheckResourceAttrSet("uptimeify_im_alert_source.s", "bearer_token"),
					tfresource.TestCheckResourceAttrSet("uptimeify_im_alert_source.s", "ingest_token"),
					tfresource.TestCheckResourceAttrSet("uptimeify_im_alert_source.s", "ingest_url"),
					tfresource.TestCheckNoResourceAttr("uptimeify_im_alert_source.s", "hmac_secret_wo"),
					tfresource.TestCheckResourceAttr("uptimeify_im_alert_source.s", "payload_mapping.attributes.1.steps.1.entries.0.to", "sev1"),
					tfresource.TestCheckResourceAttr("uptimeify_im_alert_source.s", "maintenance_windows.0.days_of_week.1", "7"),
					tfresource.TestCheckNoResourceAttr("uptimeify_im_alert_source.z", "payload_mapping"),
					func(s *terraform.State) error {
						srcID = s.RootModule().Resources["uptimeify_im_alert_source.s"].Primary.ID
						zID = s.RootModule().Resources["uptimeify_im_alert_source.z"].Primary.ID
						firstToken = attr(s, "uptimeify_im_alert_source.s", "ingest_token")
						firstBearer = attr(s, "uptimeify_im_alert_source.s", "bearer_token")
						if u := attr(s, "uptimeify_im_alert_source.s", "ingest_url"); u != os.Getenv("UPTIMEIFY_ENDPOINT")+"/api/im/ingest/"+firstToken {
							return fmt.Errorf("unexpected ingest_url %q", u)
						}
						return nil
					},
				),
			},
			{ // update in place: v1 mapping, HMAC off, rotations, maintenance cleared; data sources
				Config: alertSourceConfig(team, name, "update"),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_im_alert_source.s", "name", name+"-2"),
					tfresource.TestCheckResourceAttr("uptimeify_im_alert_source.s", "auto_resolve", "false"),
					tfresource.TestCheckResourceAttr("uptimeify_im_alert_source.s", "hmac_enabled", "false"),
					tfresource.TestCheckResourceAttr("uptimeify_im_alert_source.s", "url_token_enabled", "true"),
					tfresource.TestCheckNoResourceAttr("uptimeify_im_alert_source.s", "grouping_window_minutes"),
					tfresource.TestCheckNoResourceAttr("uptimeify_im_alert_source.s", "maintenance_windows"),
					tfresource.TestCheckNoResourceAttr("uptimeify_im_alert_source.s", "payload_mapping.attributes"),
					tfresource.TestCheckResourceAttr("uptimeify_im_alert_source.s", "payload_mapping.severity_map.crit", "sev1"),
					tfresource.TestCheckResourceAttrSet("uptimeify_im_alert_source.s", "secondary_expires_at"),
					func(s *terraform.State) error {
						if got := s.RootModule().Resources["uptimeify_im_alert_source.s"].Primary.ID; got != srcID {
							return fmt.Errorf("source was replaced: %s -> %s", srcID, got)
						}
						if attr(s, "uptimeify_im_alert_source.s", "ingest_token") == firstToken {
							return fmt.Errorf("ingest token not rotated")
						}
						if attr(s, "uptimeify_im_alert_source.s", "bearer_token") == firstBearer {
							return fmt.Errorf("bearer token not rotated")
						}
						return nil
					},
					tfresource.TestCheckResourceAttrPair("data.uptimeify_im_alert_source.by_name", "id", "uptimeify_im_alert_source.s", "id"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_im_alert_source.by_id", "payload_mapping.title", "uptimeify_im_alert_source.s", "payload_mapping.title"),
					tfresource.TestCheckResourceAttr("data.uptimeify_im_alert_source.by_id", "bearer_enabled", "true"),
					tfresource.TestCheckResourceAttr("data.uptimeify_im_alert_source.z", "payload_mapping.title", "trigger_name"),
					tfresource.TestCheckResourceAttr("data.uptimeify_im_alert_source.z", "payload_mapping.severity_map.Disaster", "sev1"),
				),
			},
			{ // import verify: the tokens are shown by the API only once
				ResourceName:      "uptimeify_im_alert_source.s",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"ingest_token", "ingest_url", "inbound_email_address", "bearer_token",
					"hmac_secret_wo_version", "ingest_token_rotation", "bearer_token_rotation"},
			},
			{ // drift: deleted outside Terraform -> plan not empty
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteAlertSource(context.Background(), srcID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             alertSourceConfig(team, name, "update"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // recreate, then the framework destroys and CheckDestroy verifies
				Config: alertSourceConfig(team, name, "update"),
				Check: func(s *terraform.State) error {
					srcID = s.RootModule().Resources["uptimeify_im_alert_source.s"].Primary.ID
					return nil
				},
			},
		},
	})
}

func routingRuleConfig(team int64, name, match string, extra string) string {
	return imProviderBlock() + fmt.Sprintf(`
resource "uptimeify_im_alert_source" "s" {
  team_id = %d
  name    = %q
  type    = "api"
}

resource "uptimeify_im_routing_rule" "r" {
  team_id = %d
  %s
  %s
}
`, team, name, team, match, extra)
}

func TestAccImRoutingRule(t *testing.T) {
	team := accImTeam(t)
	name := fmt.Sprintf("tfacc-route-%d", time.Now().Unix()%1_000_000)
	var ruleID string
	createMatch := `match = {
    source_ids    = [tonumber(uptimeify_im_alert_source.s.id)]
    severity      = ["sev1", "sev2"]
    field_matches = { env = "prod" }
  }`
	updateMatch := `match = { tags = ["db"] }`

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			if _, err := accClient().GetRoutingRule(context.Background(), ruleID); !client.IsNotFound(err) {
				return fmt.Errorf("routing rule %s still exists (err=%v)", ruleID, err)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{ // create
				Config: routingRuleConfig(team, name, createMatch, `priority = 10
  severity_override = "sev1"`),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_im_routing_rule.r", "priority", "10"),
					tfresource.TestCheckResourceAttr("uptimeify_im_routing_rule.r", "severity_override", "sev1"),
					tfresource.TestCheckResourceAttr("uptimeify_im_routing_rule.r", "match.severity.1", "sev2"),
					tfresource.TestCheckResourceAttr("uptimeify_im_routing_rule.r", "match.field_matches.env", "prod"),
					tfresource.TestCheckResourceAttrPair("uptimeify_im_routing_rule.r", "match.source_ids.0", "uptimeify_im_alert_source.s", "id"),
					func(s *terraform.State) error {
						ruleID = s.RootModule().Resources["uptimeify_im_routing_rule.r"].Primary.ID
						return nil
					},
				),
			},
			{ // update in place + data source
				Config: routingRuleConfig(team, name, updateMatch, "priority = 20") + `
data "uptimeify_im_routing_rule" "by_id" { id = uptimeify_im_routing_rule.r.id }
`,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_im_routing_rule.r", "priority", "20"),
					tfresource.TestCheckNoResourceAttr("uptimeify_im_routing_rule.r", "severity_override"),
					tfresource.TestCheckNoResourceAttr("uptimeify_im_routing_rule.r", "match.source_ids"),
					tfresource.TestCheckResourceAttr("uptimeify_im_routing_rule.r", "match.tags.0", "db"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_im_routing_rule.by_id", "match.tags.0", "uptimeify_im_routing_rule.r", "match.tags.0"),
					func(s *terraform.State) error {
						if got := s.RootModule().Resources["uptimeify_im_routing_rule.r"].Primary.ID; got != ruleID {
							return fmt.Errorf("rule was replaced: %s -> %s", ruleID, got)
						}
						return nil
					},
				),
			},
			{ // import verify
				ResourceName:      "uptimeify_im_routing_rule.r",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // catch-all: match removed, default priority
				Config: routingRuleConfig(team, name, "", ""),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckNoResourceAttr("uptimeify_im_routing_rule.r", "match"),
					tfresource.TestCheckResourceAttr("uptimeify_im_routing_rule.r", "priority", "100"),
				),
			},
			{ // drift: deleted outside Terraform -> plan not empty
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteRoutingRule(context.Background(), ruleID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             routingRuleConfig(team, name, "", ""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // recreate, then the framework destroys and CheckDestroy verifies
				Config: routingRuleConfig(team, name, "", ""),
				Check: func(s *terraform.State) error {
					ruleID = s.RootModule().Resources["uptimeify_im_routing_rule.r"].Primary.ID
					return nil
				},
			},
		},
	})
}

func orgSettingsConfig(bridge string, overage bool, fallback *int64) string {
	fb := ""
	if fallback != nil {
		fb = fmt.Sprintf("fallback_channel_id = %d", *fallback)
	}
	return imProviderBlock() + fmt.Sprintf(`
resource "uptimeify_im_org_settings" "o" {
  monitoring_bridge = %q
  overage_enabled   = %t
  %s
}
`, bridge, overage, fb)
}

// The settings are a singleton: the test keeps the existing fallback channel and restores the
// original values afterwards (destroy does not touch them).
func TestAccImOrgSettings(t *testing.T) {
	_ = accImTeam(t) // activates Incident Management
	accPreCheck(t)
	orig, err := accClient().GetImOrgSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fb := orig.FallbackChannelID

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			ctx := context.Background()
			if _, err := accClient().GetImOrgSettings(ctx); err != nil {
				return fmt.Errorf("settings gone after destroy: %v", err)
			}
			_, err := accClient().UpdateImOrgSettings(ctx, map[string]any{
				"monitoringBridge": orig.MonitoringBridge, "overageEnabled": orig.OverageEnabled, "fallbackChannelId": orig.FallbackChannelID,
			})
			return err
		},
		Steps: []tfresource.TestStep{
			{ // create = adopt + PATCH
				Config: orgSettingsConfig("both", true, fb),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_im_org_settings.o", "monitoring_bridge", "both"),
					tfresource.TestCheckResourceAttr("uptimeify_im_org_settings.o", "overage_enabled", "true"),
					tfresource.TestCheckResourceAttr("uptimeify_im_org_settings.o", "enabled", "true"),
					tfresource.TestCheckResourceAttr("uptimeify_im_org_settings.o", "id", strconv.FormatInt(orig.OrganizationID, 10)),
				),
			},
			{ // update
				Config: orgSettingsConfig("classic_only", false, fb),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_im_org_settings.o", "monitoring_bridge", "classic_only"),
					tfresource.TestCheckResourceAttr("uptimeify_im_org_settings.o", "overage_enabled", "false"),
				),
			},
			{ // import with any id
				ResourceName:      "uptimeify_im_org_settings.o",
				ImportState:       true,
				ImportStateId:     "org",
				ImportStateVerify: true,
			},
			{ // drift: changed outside Terraform -> plan not empty
				PreConfig: func() {
					accPreCheck(t)
					if _, err := accClient().UpdateImOrgSettings(context.Background(), map[string]any{"monitoringBridge": "im_only"}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             orgSettingsConfig("classic_only", false, fb),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // converge back
				Config: orgSettingsConfig("classic_only", false, fb),
				Check:  tfresource.TestCheckResourceAttr("uptimeify_im_org_settings.o", "monitoring_bridge", "classic_only"),
			},
		},
	})
}

// fakeIMSources is an in-memory stand-in for /api/im/sources (verbatim storage, preset
// mapping for zabbix, tokens only in the create answer), enough to run real plans against.
func fakeIMSources(t *testing.T) *httptest.Server {
	var mu sync.Mutex
	rows := map[string]map[string]any{}
	next := 100
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := strings.TrimPrefix(r.URL.Path, "/api/im/sources/")
		out := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/im/sources":
			next++
			row := map[string]any{"id": next, "organizationId": 2, "teamId": body["teamId"], "name": body["name"],
				"type": body["type"], "secondaryExpiresAt": nil, "authMode": map[string]any{}, "payloadMapping": map[string]any{},
				"autoResolve": true, "groupingWindowMinutes": nil, "maintenance": nil, "snoozeUntil": nil,
				"createdAt": "2026-10-20T00:00:00.000Z", "updatedAt": "2026-10-20T00:00:00.000Z"}
			if pm, ok := body["payloadMapping"]; ok {
				row["payloadMapping"] = pm
			} else if body["type"] == "zabbix" {
				row["payloadMapping"] = map[string]any{"title": "trigger_name", "severityMap": map[string]any{"Disaster": "sev1"}, "resolveValues": []any{"OK"}}
			}
			for _, k := range []string{"autoResolve", "groupingWindowMinutes", "authMode"} {
				if v, ok := body[k]; ok {
					row[k] = v
				}
			}
			rows[strconv.Itoa(next)] = row
			resp := map[string]any{"ingestToken": "tok-" + strconv.Itoa(next)}
			for k, v := range row {
				resp[k] = v
			}
			out(resp)
		case rows[id] == nil:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet:
			out(rows[id])
		case r.Method == http.MethodPatch:
			for k, v := range body {
				rows[id][k] = v
			}
			out(rows[id])
		case r.Method == http.MethodDelete:
			delete(rows, id)
			out(map[string]any{"success": true})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
}

// Unit (real plans, needs TF_ACC_TERRAFORM_PATH, e.g. /opt/tofu/tofu from scripts/acceptance.sh):
// a preset source without payload_mapping must plan empty after apply. Acceptance bug: OpenTofu
// proposed the Optional+Computed nested payload_mapping from the (null) config children, so the
// framework saw a change, marked updated_at unknown and showed "~ hmac_secret_wo (write-only)".
// payload_mapping is therefore Optional only (unset = not managed).
func TestImAlertSourcePresetPlansEmpty(t *testing.T) {
	if os.Getenv("TF_ACC_TERRAFORM_PATH") == "" {
		t.Skip("TF_ACC_TERRAFORM_PATH not set")
	}
	if os.Getenv("TF_ACC_PROVIDER_NAMESPACE") == "" {
		t.Setenv("TF_ACC_PROVIDER_NAMESPACE", "uptimeify")
	}
	srv := fakeIMSources(t)
	defer srv.Close()
	cfg := fmt.Sprintf(`
terraform {
  required_providers {
    uptimeify = { source = "uptimeify/uptimeify" }
  }
}
provider "uptimeify" {
  endpoint  = %q
  api_token = "wsm_test"
}
resource "uptimeify_im_alert_source" "z" {
  team_id = 1
  name    = "z"
  type    = "zabbix"
}
`, srv.URL)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testAccProviders,
		Steps: []tfresource.TestStep{
			{Config: cfg, Check: tfresource.ComposeAggregateTestCheckFunc(
				tfresource.TestCheckNoResourceAttr("uptimeify_im_alert_source.z", "payload_mapping"),
				tfresource.TestCheckNoResourceAttr("uptimeify_im_alert_source.z", "hmac_secret_wo"),
				tfresource.TestCheckResourceAttr("uptimeify_im_alert_source.z", "ingest_token", "tok-101"),
			)}, // the framework also asserts the empty plan after apply
			{Config: cfg, PlanOnly: true},
			{ // an import reads the stored (preset) mapping in full
				Config:        cfg,
				ResourceName:  "uptimeify_im_alert_source.z",
				ImportState:   true,
				ImportStateId: "101",
				ImportStateCheck: func(s []*terraform.InstanceState) error {
					if got := s[0].Attributes["payload_mapping.title"]; got != "trigger_name" {
						return fmt.Errorf("imported payload_mapping.title = %q", got)
					}
					return nil
				},
			},
		},
	})
}
