package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// Unit: moving the last of two tiers to position 1 sends exactly [last, first] to reorder.
func TestIMTierMoveReorders(t *testing.T) {
	var got []int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/im/teams/7/escalations":
			_, _ = w.Write([]byte(`{"tiers":[{"id":11,"tierOrder":1},{"id":12,"tierOrder":2}],"teamSettings":{}}`))
		case "/api/im/teams/7/tiers/reorder":
			body, _ := io.ReadAll(r.Body)
			var in struct {
				TierIDs []int64 `json:"tierIds"`
			}
			_ = json.Unmarshal(body, &in)
			got = in.TierIDs
			_, _ = w.Write([]byte(`{"tiers":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	r := &imTierResource{client: client.New(srv.URL, "wsm_test", "test")}
	if err := r.moveTier(context.Background(), 7, 12, 1); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []int64{12, 11}) {
		t.Fatalf("reorder body = %v, want [12 11]", got)
	}
	got = nil
	if err := r.moveTier(context.Background(), 7, 12, 2); err != nil || got != nil {
		t.Fatalf("no-op move called reorder with %v (err %v)", got, err)
	}
}

// Unit: a schedule override deleted outside Terraform (404) leaves the state; exercises the
// schedule variant's model conversion (no team_id attribute).
func TestIMScheduleOverrideReadNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/im/schedules/5/overrides/9" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"statusCode":404,"statusMessage":"Override not found","data":{"code":"imOverrideNotFound"}}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	r := &imOverrideResource{client: client.New(srv.URL, "wsm_test", "test"), parent: "schedules"}
	var sch resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sch)
	state := tfsdk.State{Schema: sch.Schema}
	if d := state.Set(ctx, &scheduleOverrideModel{ID: types.StringValue("5/9"), ScheduleID: types.Int64Value(5), OverrideID: types.Int64Value(9),
		UserID: types.StringValue("u"), Type: types.StringValue("online"), ApplyScope: types.StringValue("this_team"),
		EscalationTierOrder: types.Int64Null(), CoveredByUserIDs: stringSetValue(nil), StartsAt: types.StringValue("2030-01-01T00:00:00Z"),
		EndsAt: types.StringValue("2030-01-02T00:00:00Z"), CreatedAt: types.StringValue("x")}); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("override still in state after 404")
	}
}

// accIMUser activates Incident Management for the token's organization (idempotent) and returns
// the email of a login IM accepts as team member (active admin, editor or responder).
func accIMUser(t *testing.T) string {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	accPreCheck(t)
	ctx := context.Background()
	if err := accClient().Do(ctx, http.MethodPost, "/api/im/activate", nil, nil); err != nil {
		t.Fatalf("activating Incident Management: %v", err)
	}
	accPreCheck(t)
	users, err := accClient().ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if u.IsActive && (u.Role == "admin" || u.Role == "editor" || u.Role == "responder") {
			return u.Email
		}
	}
	t.Fatal("the organization has no active admin, editor or responder for team membership")
	return ""
}

const accProviderBlock = `
terraform {
  required_providers {
    uptimeify = { source = "uptimeify/uptimeify" }
  }
}
`

func TestAccIMTeam(t *testing.T) {
	accIMUser(t) // activates IM
	name := accName("team")
	var teamID int64
	config := func(n, extra string) string {
		return accProviderBlock + fmt.Sprintf(`
resource "uptimeify_im_team" "t" {
  name = %q
  %s
}
`, n, extra)
	}
	settings := `
  label_color                   = "#43B1AE"
  timezone                      = "Europe/Berlin"
  escalation_repeats            = 2
  escalation_repeats_stop_mode  = "resolved"
  engagement_report_enabled     = true
  engagement_report_day_of_week = 1
  engagement_report_time        = "08:30"
`
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			accPreCheck(t)
			if _, err := accClient().GetIMTeam(context.Background(), teamID); !client.IsNotFound(err) {
				return fmt.Errorf("team %d still exists (err=%v)", teamID, err)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{ // create with settings (POST + PATCH)
				Config: config(name, settings),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_im_team.t", "timezone", "Europe/Berlin"),
					tfresource.TestCheckResourceAttr("uptimeify_im_team.t", "escalation_repeats", "2"),
					tfresource.TestCheckResourceAttr("uptimeify_im_team.t", "engagement_report_time", "08:30"),
					tfresource.TestCheckResourceAttr("uptimeify_im_team.t", "analytics_enabled", "false"),
					accCaptureInt("uptimeify_im_team.t", "id", &teamID),
				),
			},
			{ // update: rename, clear settings back to defaults; data source by name and id
				Config: config(name+"-2", `analytics_enabled = true`) + `
data "uptimeify_im_team" "by_name" { name = uptimeify_im_team.t.name }
data "uptimeify_im_team" "by_id"   { id = uptimeify_im_team.t.id }
`,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_im_team.t", "name", name+"-2"),
					tfresource.TestCheckNoResourceAttr("uptimeify_im_team.t", "timezone"),
					tfresource.TestCheckNoResourceAttr("uptimeify_im_team.t", "label_color"),
					tfresource.TestCheckResourceAttr("uptimeify_im_team.t", "escalation_repeats", "0"),
					tfresource.TestCheckResourceAttr("uptimeify_im_team.t", "analytics_enabled", "true"),
					accSameInt("uptimeify_im_team.t", "id", &teamID),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_im_team.by_name", "id", "uptimeify_im_team.t", "id"),
					tfresource.TestCheckResourceAttr("data.uptimeify_im_team.by_id", "analytics_enabled", "true"),
				),
			},
			{ResourceName: "uptimeify_im_team.t", ImportState: true, ImportStateVerify: true},
			{ // drift: deleted outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteIMTeam(context.Background(), teamID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             config(name+"-2", `analytics_enabled = true`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config(name+"-2", `analytics_enabled = true`),
				Check:  accCaptureInt("uptimeify_im_team.t", "id", &teamID),
			},
		},
	})
}

// imStack is the whole Incident Management setup of one team, in two variants.
type imStack struct {
	name, email      string
	updated          bool
	startsAt, endsAt string
}

func (s imStack) config() string {
	role, minutes, severities, tier1Order, tier2Order := "admin", 5, `auto_escalation_severities = ["critical"]`, "", 1
	sched := `rotation_mode = "none"`
	effective := ""
	teamOverrideSchedule := ""
	endsAt := s.endsAt
	if s.updated {
		role, minutes, severities, tier1Order, tier2Order = "member", 10, "", "tier_order = 1", 2
		sched = `
  rotation_mode         = "explicit"
  rotation_repeats      = "weekly"
  starts_on_day_of_week = 1
  starts_on_time        = "09:00"`
		effective = fmt.Sprintf(`effective_from = %q`, s.startsAt)
		teamOverrideSchedule = "schedule_id = uptimeify_im_schedule.s.id"
		endsAt = s.endsAt[:len(s.endsAt)-len("Z")] + "+02:00" // another spelling, a different instant
	}
	return accProviderBlock + fmt.Sprintf(`
data "uptimeify_user" "u" { email = %[2]q }

resource "uptimeify_im_team" "t" { name = %[1]q }

resource "uptimeify_im_team_member" "m" {
  team_id = uptimeify_im_team.t.id
  user_id = data.uptimeify_user.u.id
  im_role = %[3]q
}

resource "uptimeify_im_escalation_tier" "first" {
  team_id                       = uptimeify_im_team.t.id
  member_user_ids               = [uptimeify_im_team_member.m.user_id]
  auto_escalation_after_minutes = %[4]d
  %[5]s
  %[6]s
}

# Created after "first" and moved in front of it; the update swaps them back.
resource "uptimeify_im_escalation_tier" "second" {
  team_id         = uptimeify_im_team.t.id
  member_user_ids = [uptimeify_im_team_member.m.user_id]
  tier_order      = %[12]d
  depends_on      = [uptimeify_im_escalation_tier.first]
}

resource "uptimeify_im_schedule" "s" {
  tier_id  = uptimeify_im_escalation_tier.first.tier_id
  name     = "%[1]s-schedule"
  timezone = "Europe/Berlin"
  %[7]s
  %[8]s
  weekly_schedules = [{ from = "08:00", until = "24:00", days = [1, 2, 3, 4, 5] }]
  rotations        = [{ members = [uptimeify_im_team_member.m.user_id] }]
}

resource "uptimeify_im_schedule_override" "o" {
  schedule_id = uptimeify_im_schedule.s.id
  user_id     = data.uptimeify_user.u.id
  type        = "offline"
  starts_at   = %[9]q
  ends_at     = %[10]q
}

resource "uptimeify_im_team_override" "o" {
  team_id   = uptimeify_im_team.t.id
  user_id   = data.uptimeify_user.u.id
  type      = "online"
  starts_at = %[9]q
  ends_at   = %[10]q
  %[11]s
}
`, s.name, s.email, role, minutes, severities, tier1Order, sched, effective, s.startsAt, endsAt, teamOverrideSchedule, tier2Order)
}

func TestAccIMStack(t *testing.T) {
	email := accIMUser(t)
	start := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Hour)
	s := imStack{name: accName("im"), email: email,
		startsAt: start.Format(time.RFC3339), endsAt: start.Add(48 * time.Hour).Format(time.RFC3339)}
	upd := s
	upd.updated = true

	var teamID, scheduleID int64
	const (
		tier1 = "uptimeify_im_escalation_tier.first"
		tier2 = "uptimeify_im_escalation_tier.second"
		sched = "uptimeify_im_schedule.s"
	)
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			accPreCheck(t)
			ctx := context.Background()
			if _, err := accClient().GetIMSchedule(ctx, scheduleID); !client.IsNotFound(err) {
				return fmt.Errorf("schedule %d still exists (err=%v)", scheduleID, err)
			}
			if _, err := accClient().GetIMTeam(ctx, teamID); !client.IsNotFound(err) {
				return fmt.Errorf("team %d still exists (err=%v)", teamID, err)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: s.config(),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					accCaptureInt("uptimeify_im_team.t", "id", &teamID),
					accCaptureInt(sched, "id", &scheduleID),
					tfresource.TestCheckResourceAttr("uptimeify_im_team_member.m", "im_role", "admin"),
					tfresource.TestCheckResourceAttrSet(tier1, "default_schedule_id"),
					tfresource.TestCheckResourceAttr(tier1, "auto_escalation_severities.#", "1"),
					tfresource.TestCheckResourceAttr(tier2, "tier_order", "1"),
					tfresource.TestCheckResourceAttr(sched, "rotation_mode", "none"),
					tfresource.TestCheckResourceAttr(sched, "rotations.0.members.#", "1"),
					tfresource.TestCheckResourceAttrPair(sched, "team_id", "uptimeify_im_team.t", "id"),
					tfresource.TestCheckResourceAttr("uptimeify_im_schedule_override.o", "ends_at", s.endsAt),
					tfresource.TestCheckResourceAttr("uptimeify_im_schedule_override.o", "apply_scope", "this_team"),
					tfresource.TestCheckNoResourceAttr("uptimeify_im_team_override.o", "schedule_id"),
				),
			},
			{ // in-place updates, incl. reordering: first -> 1, second -> 2
				Config: upd.config(),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					accSameInt("uptimeify_im_team.t", "id", &teamID),
					accSameInt(sched, "id", &scheduleID),
					tfresource.TestCheckResourceAttr("uptimeify_im_team_member.m", "im_role", "member"),
					tfresource.TestCheckResourceAttr(tier1, "tier_order", "1"),
					tfresource.TestCheckResourceAttr(tier2, "tier_order", "2"),
					tfresource.TestCheckResourceAttr(tier1, "auto_escalation_after_minutes", "10"),
					tfresource.TestCheckNoResourceAttr(tier1, "auto_escalation_severities"),
					tfresource.TestCheckResourceAttr(sched, "rotation_repeats", "weekly"),
					tfresource.TestCheckResourceAttr(sched, "starts_on_time", "09:00"),
					tfresource.TestCheckResourceAttr(sched, "effective_from", s.startsAt),
					tfresource.TestCheckResourceAttrPair("uptimeify_im_team_override.o", "schedule_id", sched, "id"),
				),
			},
			{ResourceName: "uptimeify_im_team_member.m", ImportState: true, ImportStateVerify: true},
			{ResourceName: tier1, ImportState: true, ImportStateVerify: true},
			{ResourceName: tier2, ImportState: true, ImportStateVerify: true},
			// Imported timestamps come back in the API's spelling (UTC, milliseconds).
			{ResourceName: sched, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"effective_from"}},
			{ResourceName: "uptimeify_im_schedule_override.o", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"starts_at", "ends_at"}},
			{ResourceName: "uptimeify_im_team_override.o", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"starts_at", "ends_at"}},
			{ // drift: schedule (and with it its override) deleted outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteIMSchedule(context.Background(), scheduleID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             upd.config(),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: upd.config(),
				Check:  accCaptureInt(sched, "id", &scheduleID),
			},
		},
	})
}

func accCaptureInt(name, attr string, dst *int64) tfresource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not in state", name)
		}
		n, err := strconv.ParseInt(rs.Primary.Attributes[attr], 10, 64)
		*dst = n
		return err
	}
}

func accSameInt(name, attr string, want *int64) tfresource.TestCheckFunc {
	return func(s *terraform.State) error {
		var got int64
		if err := accCaptureInt(name, attr, &got)(s); err != nil {
			return err
		}
		if got != *want {
			return fmt.Errorf("%s was replaced: %s %d -> %d", name, attr, *want, got)
		}
		return nil
	}
}
