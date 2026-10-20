package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

func TestKeepSameInstant(t *testing.T) {
	api := "2030-01-01T02:00:00.000Z"
	if got := keepSameInstant(types.StringValue("2030-01-01T03:00:00+01:00"), api); got.ValueString() != "2030-01-01T03:00:00+01:00" {
		t.Fatalf("same instant not kept: %s", got)
	}
	if got := keepSameInstant(types.StringValue("2030-01-01T04:00:00Z"), api); got.ValueString() != api {
		t.Fatalf("drift hidden: %s", got)
	}
	if got := keepSameInstant(types.StringNull(), api); got.ValueString() != api {
		t.Fatalf("import: %s", got)
	}
}

func TestMaintenancePatchOnlyChangedAndTargetsWithTags(t *testing.T) {
	state := maintenanceWindowModel{
		Name: types.StringValue("a"), Description: types.StringValue("d"),
		StartTime: types.StringValue("2030-01-01T00:00:00Z"), EndTime: types.StringValue("2030-01-01T01:00:00Z"),
		Timezone: types.StringValue("UTC"), IsActive: types.BoolValue(true), TagIDs: []int64{1, 2},
	}
	plan := state
	plan.TagIDs = []int64{2, 1}
	if p := maintenancePatch(&plan, &state); len(p) != 0 {
		t.Fatalf("reordered tags must not patch: %v", p)
	}

	plan.Description = types.StringNull()
	plan.TagIDs = []int64{3}
	plan.RecurrencePattern = &recurrenceModel{Frequency: types.StringValue("weekly"), IntervalCount: types.Int64Value(2), DaysOfWeek: []int64{1}}
	p := maintenancePatch(&plan, &state)
	if len(p) != 5 || p["description"].(*string) != nil || p["isRecurring"] != true {
		t.Fatalf("patch %v", p)
	}
	if tg := p["targets"].([]client.MaintenanceTarget); len(tg) != 0 || tg == nil {
		t.Fatalf("targets must travel with tags as [], got %#v", p["targets"])
	}
	if rp := p["recurrencePattern"].(*client.RecurrencePattern); *rp.IntervalCount != 2 || rp.Frequency != "weekly" {
		t.Fatalf("recurrence %+v", rp)
	}
}

func TestMaintenanceFromAPICustomerOnlyForWholeCustomerWindow(t *testing.T) {
	cust := int64(7)
	var m maintenanceWindowModel
	m.fromAPI(&client.MaintenanceWindow{ID: 1, CustomerID: &cust, Tags: []client.Tag{{ID: 3}}}, &maintenanceWindowModel{})
	if !m.CustomerID.IsNull() || len(m.TagIDs) != 1 {
		t.Fatalf("tag window: customer_id %s tags %v", m.CustomerID, m.TagIDs)
	}
	m.fromAPI(&client.MaintenanceWindow{ID: 1, CustomerID: &cust, IsRecurring: false,
		RecurrencePattern: &client.RecurrencePattern{Frequency: "daily"}}, &maintenanceWindowModel{})
	if m.CustomerID.ValueInt64() != 7 || m.RecurrencePattern != nil || m.Targets != nil {
		t.Fatalf("customer window: %+v", m)
	}
}

func TestMaintenanceReadNotFoundRemovesResource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"statusCode":404,"statusMessage":"Maintenance window not found","data":{"code":"maintenanceWindowNotFound"}}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	r := &maintenanceWindowResource{client: client.New(srv.URL, "wsm_test", "test")}
	var sch resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sch)
	state := tfsdk.State{Schema: sch.Schema}
	if d := state.Set(ctx, &maintenanceWindowModel{ID: types.StringValue("9"), Name: types.StringValue("x"),
		StartTime: types.StringValue("2030-01-01T00:00:00Z"), EndTime: types.StringValue("2030-01-01T01:00:00Z"),
		Timezone: types.StringValue("UTC"), IsActive: types.BoolValue(true), TagIDs: []int64{1}}); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() || !resp.State.Raw.IsNull() {
		t.Fatalf("diags=%v removed=%v", resp.Diagnostics, resp.State.Raw.IsNull())
	}
}

// Acceptance: a window on a tag (created here), updated to a weekly recurrence.
func TestAccMaintenanceWindow(t *testing.T) {
	suffix := time.Now().Unix() % 1_000_000
	name := fmt.Sprintf("tfacc-mw-%d", suffix)
	tag := fmt.Sprintf("tfacc-mwt-%d", suffix)
	var windowID string

	tfresource.Test(t, tfresource.TestCase{
		PreCheck: func() {
			if os.Getenv("UPTIMEIFY_API_TOKEN") == "" {
				t.Fatal("UPTIMEIFY_API_TOKEN must be set for acceptance tests")
			}
		},
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			if _, err := accClient().GetMaintenanceWindow(context.Background(), windowID); !client.IsNotFound(err) {
				return fmt.Errorf("maintenance window %s still exists (err=%v)", windowID, err)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{ // create: one-off window on a tag
				Config: maintenanceConfig(tag, name, ""),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_maintenance_window.w", "name", name),
					tfresource.TestCheckResourceAttr("uptimeify_maintenance_window.w", "timezone", "UTC"),
					tfresource.TestCheckResourceAttr("uptimeify_maintenance_window.w", "is_active", "true"),
					tfresource.TestCheckResourceAttr("uptimeify_maintenance_window.w", "tag_ids.#", "1"),
					tfresource.TestCheckTypeSetElemAttrPair("uptimeify_maintenance_window.w", "tag_ids.*", "uptimeify_tag.t", "id"),
					tfresource.TestCheckNoResourceAttr("uptimeify_maintenance_window.w", "recurrence_pattern"),
					tfresource.TestCheckNoResourceAttr("uptimeify_maintenance_window.w", "customer_id"),
					tfresource.TestCheckResourceAttrSet("uptimeify_maintenance_window.w", "public_id"),
					func(s *terraform.State) error {
						windowID = s.RootModule().Resources["uptimeify_maintenance_window.w"].Primary.ID
						return nil
					},
				),
			},
			{ // update in place: weekly recurrence, Berlin time, description, inactive + data source
				Config: maintenanceConfig(tag, name+"-2", `
  description = "tfacc update"
  timezone    = "Europe/Berlin"
  is_active   = false
  recurrence_pattern = {
    frequency           = "weekly"
    interval_count      = 2
    days_of_week        = [1, 3]
    end_recurrence_date = "2031-12-31"
  }
`) + `
data "uptimeify_maintenance_window" "by_id" { id = uptimeify_maintenance_window.w.public_id }
`,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_maintenance_window.w", "name", name+"-2"),
					tfresource.TestCheckResourceAttr("uptimeify_maintenance_window.w", "timezone", "Europe/Berlin"),
					tfresource.TestCheckResourceAttr("uptimeify_maintenance_window.w", "is_active", "false"),
					tfresource.TestCheckResourceAttr("uptimeify_maintenance_window.w", "recurrence_pattern.interval_count", "2"),
					tfresource.TestCheckResourceAttr("uptimeify_maintenance_window.w", "recurrence_pattern.days_of_week.1", "3"),
					func(s *terraform.State) error {
						if got := s.RootModule().Resources["uptimeify_maintenance_window.w"].Primary.ID; got != windowID {
							return fmt.Errorf("window was replaced: %s -> %s", windowID, got)
						}
						return nil
					},
					tfresource.TestCheckResourceAttrPair("data.uptimeify_maintenance_window.by_id", "name", "uptimeify_maintenance_window.w", "name"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_maintenance_window.by_id", "recurrence_pattern.frequency", "uptimeify_maintenance_window.w", "recurrence_pattern.frequency"),
					tfresource.TestCheckResourceAttr("data.uptimeify_maintenance_window.by_id", "tag_ids.#", "1"),
				),
			},
			{ // import verify
				ResourceName:      "uptimeify_maintenance_window.w",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // drift: deleted outside Terraform -> plan not empty
				PreConfig: func() {
					if err := accClient().DeleteMaintenanceWindow(context.Background(), windowID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             maintenanceConfig(tag, name+"-2", ""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // recreate (back to one-off), then destroy + CheckDestroy
				Config: maintenanceConfig(tag, name+"-2", ""),
				Check: func(s *terraform.State) error {
					windowID = s.RootModule().Resources["uptimeify_maintenance_window.w"].Primary.ID
					return nil
				},
			},
		},
	})
}

// maintenanceConfig uses the API's canonical time form so ImportStateVerify compares equal.
func maintenanceConfig(tag, name, extra string) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    uptimeify = { source = "uptimeify/uptimeify" }
  }
}

resource "uptimeify_tag" "t" {
  name = %q
}

resource "uptimeify_maintenance_window" "w" {
  name       = %q
  start_time = "2030-01-01T02:00:00.000Z"
  end_time   = "2030-01-01T04:00:00.000Z"
  tag_ids    = [tonumber(uptimeify_tag.t.id)]
  %s
}
`, tag, name, extra)
}
