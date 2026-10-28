package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// Unit: the API answers UTC with milliseconds; the configured spelling of the same instant stays.
func TestStatusPageMaintenanceKeepInstant(t *testing.T) {
	cfg := types.StringValue("2026-11-03T22:00:00+01:00")
	if got := keepInstant(cfg, "2026-11-03T21:00:00.000Z"); got.ValueString() != cfg.ValueString() {
		t.Fatalf("same instant lost the configured spelling: %v", got)
	}
	if got := keepInstant(cfg, "2026-11-03T21:30:00.000Z"); got.ValueString() != "2026-11-03T21:30:00.000Z" {
		t.Fatalf("drift not shown: %v", got)
	}
	if got := keepInstant(types.StringNull(), "2026-11-03T21:00:00.000Z"); got.ValueString() != "2026-11-03T21:00:00.000Z" {
		t.Fatalf("import must take the API value: %v", got)
	}
}

// Unit: page-wide answers componentIds [] and stays unset; a component list is kept.
func TestStatusPageMaintenanceFromAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status-pages/maintenances/abc-1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"id":7,"publicId":"abc-1","title":"DB","scheduledFor":"2026-11-03T21:00:00.000Z",
			"scheduledUntil":"2026-11-03T22:00:00.000Z","state":"scheduled","notifyOnSchedule":true,"reminderMinutesBefore":null,
			"notifyOnStart":false,"notifyOnComplete":true,"cancelledAt":null,"statusPageIds":[4],"componentIds":[]}`))
	}))
	defer srv.Close()
	c := client.New(srv.URL, "wsm_test", "test")
	x, err := c.GetStatusPageMaintenance(context.Background(), "abc-1")
	if err != nil {
		t.Fatal(err)
	}
	m := statusPageMaintenanceModel{ComponentIDs: types.SetNull(types.Int64Type)}
	if d := m.fromAPI(context.Background(), x); d.HasError() {
		t.Fatal(d)
	}
	if m.ID.ValueString() != "7" || !m.ComponentIDs.IsNull() || !m.ReminderMinutesBefore.IsNull() || len(m.StatusPageIDs.Elements()) != 1 {
		t.Fatalf("got %+v", m)
	}
	if _, err := c.GetStatusPageMaintenance(context.Background(), "nope"); !client.IsNotFound(err) {
		t.Fatalf("want 404, got %v", err)
	}
}

// Acceptance: a maintenance on an organization page (no customer) and one of its components:
// create, update in place (title, end, reminder, page-wide), import, drift, destroy.
func TestAccStatusPageMaintenance(t *testing.T) {
	spAccSkip(t)
	name := fmt.Sprintf("tfacc-spm-%d", time.Now().UnixNano()%1_000_000)
	const m = "uptimeify_status_page_maintenance.m"
	start := time.Now().Add(48 * time.Hour).Truncate(time.Hour).In(time.FixedZone("CET", 3600))
	ts := func(t time.Time) string { return t.Format(time.RFC3339) }
	var pageID, maintID string
	cfg := func(title string, end time.Time, extra string) string {
		return fmt.Sprintf(`
resource "uptimeify_status_page" "p" {
  name                      = %[1]q
  slug                      = %[1]q
  include_customer_monitors = false
}

resource "uptimeify_status_page_component" "api" {
  status_page_id = tonumber(uptimeify_status_page.p.id)
  source_kind    = "manual"
  name           = "API"
}

resource "uptimeify_status_page_maintenance" "m" {
  title           = %[2]q
  scheduled_for   = %[3]q
  scheduled_until = %[4]q
  status_page_ids = [tonumber(uptimeify_status_page.p.id)]
  notify_on_schedule = false
  %[5]s
}
`, name, title, ts(start), ts(end), extra)
	}
	step1 := cfg(name+" DB upgrade", start.Add(time.Hour), `component_ids = [uptimeify_status_page_component.api.component_id]
  description   = "Short read-only phase"`)
	step2 := cfg(name+" DB upgrade (extended)", start.Add(2*time.Hour), `reminder_minutes_before = 60
  notify_on_start         = true`)

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(s *terraform.State) error {
			if _, err := accClient().GetStatusPageMaintenance(context.Background(), maintID); !client.IsNotFound(err) {
				return fmt.Errorf("maintenance %s still exists (err=%v)", maintID, err)
			}
			return statusPageGone(&pageID)(s)
		},
		Steps: []tfresource.TestStep{
			{
				Config: step1,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSaveID("uptimeify_status_page.p", &pageID),
					spSaveID(m, &maintID),
					tfresource.TestCheckNoResourceAttr("uptimeify_status_page.p", "customer_id"),
					tfresource.TestCheckResourceAttr(m, "state", "scheduled"),
					tfresource.TestCheckResourceAttr(m, "scheduled_for", ts(start)),
					tfresource.TestCheckResourceAttr(m, "component_ids.#", "1"),
					tfresource.TestCheckTypeSetElemAttrPair(m, "component_ids.*", "uptimeify_status_page_component.api", "component_id"),
					tfresource.TestCheckResourceAttr(m, "description", "Short read-only phase"),
					tfresource.TestCheckResourceAttr(m, "notify_on_complete", "true"),
					tfresource.TestCheckNoResourceAttr(m, "reminder_minutes_before"),
					tfresource.TestCheckNoResourceAttr(m, "cancelled_at"),
				),
			},
			{ // update in place: page-wide, longer, reminder, description cleared
				Config: step2,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSameID(m, &maintID),
					tfresource.TestCheckResourceAttr(m, "title", name+" DB upgrade (extended)"),
					tfresource.TestCheckResourceAttr(m, "scheduled_until", ts(start.Add(2*time.Hour))),
					tfresource.TestCheckResourceAttr(m, "reminder_minutes_before", "60"),
					tfresource.TestCheckResourceAttr(m, "notify_on_start", "true"),
					tfresource.TestCheckNoResourceAttr(m, "component_ids"),
					tfresource.TestCheckNoResourceAttr(m, "description"),
					func(*terraform.State) error {
						x, err := accClient().GetStatusPageMaintenance(context.Background(), maintID)
						if err != nil {
							return err
						}
						if len(x.ComponentIDs) != 0 || x.Description != nil || !sameInstant(x.ScheduledUntil, ts(start.Add(2*time.Hour))) {
							return fmt.Errorf("not written on the server: %+v", x)
						}
						return nil
					},
				),
			},
			{Config: step2, PlanOnly: true}, // the offset spelling does not drift against the UTC answer
			{ResourceName: m, ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"scheduled_for", "scheduled_until"}},
			{ // drift: title changed outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if _, err := accClient().UpdateStatusPageMaintenance(context.Background(), maintID, map[string]any{"title": "changed elsewhere"}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             step2,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: step2,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSameID(m, &maintID),
					tfresource.TestCheckResourceAttr(m, "title", name+" DB upgrade (extended)"),
				),
			},
			{ // drift: deleted outside Terraform, plan recreates it
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteStatusPageMaintenance(context.Background(), maintID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             step2,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: step2,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSaveID(m, &maintID),
					tfresource.TestCheckResourceAttr(m, "state", "scheduled"),
				),
			},
		},
	})
}
