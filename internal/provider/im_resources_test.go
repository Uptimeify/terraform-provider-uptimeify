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

// Unit: the schedule's rotations come from GET /api/im/schedules/{id} itself, no tier read.
func TestIMScheduleReadsRotationsFromScheduleGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/im/schedules/5" {
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"id":5,"teamId":7,"tierId":21,"name":"s","timezone":"UTC","weeklySchedules":[],"rotationMode":"none","roundRobinSize":1,
			"rotations":[{"id":1,"rotationOrder":1,"members":[{"userId":"a","memberOrder":0},{"userId":"b","memberOrder":1}]},
			{"id":2,"rotationOrder":2,"members":[{"userId":"c","memberOrder":0}]}]}`))
	}))
	defer srv.Close()
	r := &imScheduleResource{client: client.New(srv.URL, "wsm_test", "test")}
	var m imScheduleModel
	if _, err := r.readInto(context.Background(), 5, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Rotations) != 2 || len(m.Rotations[0].Members) != 2 || m.Rotations[0].Members[1].ValueString() != "b" || m.Rotations[1].Members[0].ValueString() != "c" {
		t.Fatalf("rotations = %+v", m.Rotations)
	}
}

// Unit: create sends the team settings in the POST body (no follow-up PATCH).
func TestIMTeamCreateBodyIsFull(t *testing.T) {
	plan := imTeamModel{Name: types.StringValue("t"), LabelColor: types.StringNull(), AnalyticsEnabled: types.BoolValue(false),
		Timezone: types.StringValue("Europe/Berlin"), EscalationRepeats: types.Int64Value(2), EscalationRepeatsAfterMinutes: types.Int64Value(10),
		EscalationRepeatsStopMode: types.StringValue("resolved"), EngagementReportEnabled: types.BoolValue(true),
		EngagementReportDayOfWeek: types.Int64Value(1), EngagementReportTime: types.StringValue("09:00")}
	got := imTeamPatch(&plan, nil)
	for _, k := range []string{"name", "labelColor", "analyticsEnabled", "timezone", "escalationRepeats", "escalationRepeatsAfterMinutes",
		"escalationRepeatsStopMode", "engagementReportEnabled", "engagementReportDayOfWeek", "engagementReportTime"} {
		if _, ok := got[k]; !ok {
			t.Errorf("create body misses %s", k)
		}
	}
	cur := plan
	cur.Timezone = types.StringValue("UTC")
	if p := imTeamPatch(&plan, &cur); len(p) != 1 || p["timezone"] == nil {
		t.Fatalf("patch = %v, want only timezone", p)
	}
}

// Unit: an override PATCH carries only the changed fields; once userId, type, startsAt and endsAt
// all change it is the full override (the API's full-replace form resets unsent fields).
func TestIMOverridePatch(t *testing.T) {
	ctx := context.Background()
	cur := imOverrideModel{TeamID: types.Int64Value(7), ScheduleID: types.Int64Value(5), UserID: types.StringValue("u"), Type: types.StringValue("offline"),
		ApplyScope: types.StringValue("this_team"), EscalationTierOrder: types.Int64Value(2), CoveredByUserIDs: stringSetValue([]string{"x", "y"}),
		StartsAt: types.StringValue("2030-01-01T00:00:00Z"), EndsAt: types.StringValue("2030-01-02T00:00:00Z")}
	plan := cur
	plan.EndsAt = types.StringValue("2030-01-03T00:00:00Z")
	plan.EscalationTierOrder = types.Int64Null()
	p := plan.patch(ctx, true, &cur)
	if len(p) != 2 || p["endsAt"] != "2030-01-03T00:00:00Z" || p["escalationTierOrder"].(*int64) != nil {
		t.Fatalf("partial patch = %v", p)
	}
	if _, ok := p["escalationTierOrder"]; !ok {
		t.Fatal("cleared escalationTierOrder must go out as null")
	}
	if p := cur.patch(ctx, true, &cur); len(p) != 0 {
		t.Fatalf("unchanged override patch = %v", p)
	}
	plan = cur
	plan.UserID, plan.Type = types.StringValue("v"), types.StringValue("online")
	plan.CoveredByUserIDs = stringSetValue(nil)
	plan.StartsAt, plan.EndsAt = types.StringValue("2031-01-01T00:00:00Z"), types.StringValue("2031-01-02T00:00:00Z")
	if p := plan.patch(ctx, true, &cur); len(p) != 8 || p["applyScope"] != "this_team" || p["scheduleId"].(*int64) == nil {
		t.Fatalf("full-replace patch = %v", p)
	}
}

// Unit: create sends tier_order in the POST; a position past the end (422 imInvalidNumber) is
// appended instead, without a reorder call.
func TestIMTierCreateTierOrder(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/im/teams/7/tiers":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			bodies = append(bodies, in)
			if _, ok := in["tierOrder"]; ok {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"statusCode":422,"statusMessage":"tierOrder must be between 1 and 1","data":{"code":"imInvalidNumber"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":21,"tierOrder":1,"autoEscalationEnabled":true,"autoEscalationAfterMinutes":5,"autoEscalationStopMode":"acknowledged",
				"autoEscalationTimeFilters":[],"repeats":0,"repeatsStopMode":"acknowledged","schedules":[{"id":31}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/im/teams/7/tiers/21":
			_, _ = w.Write([]byte(`{"id":21,"tierOrder":1,"autoEscalationEnabled":true,"autoEscalationAfterMinutes":5,"autoEscalationStopMode":"acknowledged",
				"autoEscalationTimeFilters":[],"repeats":0,"repeatsStopMode":"acknowledged",
				"schedules":[{"id":31,"rotations":[{"id":1,"rotationOrder":1,"members":[{"userId":"u","memberOrder":0}]}]}]}`))
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	r := &imTierResource{client: client.New(srv.URL, "wsm_test", "test")}
	var sch resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sch)
	plan := tfsdk.Plan{Schema: sch.Schema}
	if d := plan.Set(ctx, &imTierModel{ID: types.StringUnknown(), TeamID: types.Int64Value(7), TierID: types.Int64Unknown(), TierOrder: types.Int64Value(3),
		MemberUserIDs: []types.String{types.StringValue("u")}, DefaultScheduleID: types.Int64Unknown(), AutoEscalationEnabled: types.BoolValue(true),
		AutoEscalationAfterMinutes: types.Int64Value(5), AutoEscalationStopMode: types.StringValue("acknowledged"), AutoEscalationSeverities: types.SetNull(types.StringType),
		AutoEscalationTimeFilters: []weeklyWindowModel{}, Repeats: types.Int64Value(0), RepeatsAfterMinutes: types.Int64Null(),
		RepeatsStopMode: types.StringValue("acknowledged")}); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if len(bodies) != 2 || bodies[0]["tierOrder"] != float64(3) {
		t.Fatalf("POST bodies = %v, want tierOrder 3 then append", bodies)
	}
	var got imTierModel
	resp.State.Get(ctx, &got)
	if got.TierOrder.ValueInt64() != 1 || got.DefaultScheduleID.ValueInt64() != 31 {
		t.Fatalf("state tier_order %v default_schedule_id %v", got.TierOrder, got.DefaultScheduleID)
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
