package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// Unit: the API answer lands in the model, monitorName stays out of the set, and a PATCH carries
// only what changed: cleared fields as JSON null, an emptied set as [].
func TestIMServiceBody(t *testing.T) {
	ctx := context.Background()
	var api client.IMService
	if err := json.Unmarshal([]byte(`{"id":7,"publicId":"p","organizationId":2,"name":"Shop","description":"d","criticality":"high","ownerTeamId":3,
		"monitors":[{"monitorType":"tcp","monitorId":5,"monitorName":"x"}],"createdAt":"c","updatedAt":"u"}`), &api); err != nil {
		t.Fatal(err)
	}
	var cur imServiceModel
	cur.fromAPI(&api)
	if cur.ID.ValueString() != "7" || len(cur.Monitors.Elements()) != 1 {
		t.Fatalf("model %+v", cur)
	}

	same := cur
	if body, d := imServiceBody(ctx, &same, &cur); d.HasError() || len(body) != 0 {
		t.Fatalf("unchanged plan sent %v (%v)", body, d)
	}

	cleared := cur
	cleared.Description = types.StringNull()
	cleared.OwnerTeamID = types.Int64Null()
	cleared.Monitors = types.SetValueMust(imServiceMonitorType, nil)
	body, d := imServiceBody(ctx, &cleared, &cur)
	if d.HasError() {
		t.Fatal(d)
	}
	raw, _ := json.Marshal(body)
	if want := `{"description":null,"monitors":[],"ownerTeamId":null}`; string(raw) != want {
		t.Fatalf("patch = %s, want %s", raw, want)
	}

	body, d = imServiceBody(ctx, &cur, nil)
	if d.HasError() {
		t.Fatal(d)
	}
	raw, _ = json.Marshal(body)
	if want := `{"criticality":"high","description":"d","monitors":[{"monitorType":"tcp","monitorId":5}],"name":"Shop","ownerTeamId":3}`; string(raw) != want {
		t.Fatalf("create = %s, want %s", raw, want)
	}
}

// Unit: a service deleted outside Terraform (404) leaves the state.
func TestIMServiceReadNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/im/services/7" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"statusCode":404,"statusMessage":"Service not found","data":{"code":"imServiceNotFound"}}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	r := &imServiceResource{client: client.New(srv.URL, "wsm_test", "test")}
	var sch resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sch)
	state := tfsdk.State{Schema: sch.Schema}
	var m imServiceModel
	m.fromAPI(&client.IMService{ID: 7, Name: "x", Criticality: "low"})
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

// Unit: a name shared across teams needs team_id; no match and several matches both fail.
func TestFindIMSchedule(t *testing.T) {
	items := []client.IMScheduleListItem{{ID: 1, TeamID: 10, Name: "Tier 1 schedule"}, {ID: 2, TeamID: 20, Name: "Tier 1 schedule"}, {ID: 3, TeamID: 20, Name: "Nights"}}
	if id, err := findIMSchedule(items, "Nights", 0); err != nil || id != 3 {
		t.Fatalf("unique name: %d, %v", id, err)
	}
	if id, err := findIMSchedule(items, "Tier 1 schedule", 20); err != nil || id != 2 {
		t.Fatalf("name in team: %d, %v", id, err)
	}
	if _, err := findIMSchedule(items, "Tier 1 schedule", 0); err == nil || !strings.Contains(err.Error(), "2 schedules") {
		t.Fatalf("ambiguous name: %v", err)
	}
	if _, err := findIMSchedule(items, "Nights", 10); err == nil {
		t.Fatal("name of another team matched")
	}
}

// Acceptance: UPTIMEIFY_ACC_MONITOR_ID = numeric id of a website of the token's organization.
func TestAccIMService(t *testing.T) {
	accIMUser(t) // activates IM
	monitorID := os.Getenv("UPTIMEIFY_ACC_MONITOR_ID")
	if monitorID == "" {
		t.Skip("UPTIMEIFY_ACC_MONITOR_ID not set: the service test needs the numeric id of an existing website")
	}
	const r = "uptimeify_im_service.s"
	name := accName("svc")
	config := func(n, attrs string) string {
		return accProviderBlock + fmt.Sprintf(`
resource "uptimeify_im_team" "t" { name = %q }

resource "uptimeify_im_service" "s" {
  name = %q
  %s
}
`, name+"-team", n, attrs)
	}
	full := fmt.Sprintf(`
  description   = "tfacc line one\nline two"
  criticality   = "critical"
  owner_team_id = tonumber(uptimeify_im_team.t.id)
  monitors      = [{ monitor_type = "website", monitor_id = %s }]`, monitorID)
	var id int64
	serviceID := func() string { return fmt.Sprint(id) }
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			accPreCheck(t)
			if _, err := accClient().GetIMService(context.Background(), serviceID()); !client.IsNotFound(err) {
				return fmt.Errorf("service %d still exists (err=%v)", id, err)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{ // create with defaults
				Config: config(name, ""),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					accCaptureInt(r, "id", &id),
					tfresource.TestCheckResourceAttr(r, "criticality", "medium"),
					tfresource.TestCheckNoResourceAttr(r, "description"),
					tfresource.TestCheckNoResourceAttr(r, "owner_team_id"),
					tfresource.TestCheckResourceAttr(r, "monitors.#", "0"),
					tfresource.TestCheckResourceAttrSet(r, "public_id"),
					tfresource.TestCheckResourceAttrSet(r, "organization_id"),
					tfresource.TestCheckResourceAttrSet(r, "created_at"),
				),
			},
			{ // update in place: every field
				Config: config(name+"-2", full),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					accSameInt(r, "id", &id),
					tfresource.TestCheckResourceAttr(r, "name", name+"-2"),
					tfresource.TestCheckResourceAttr(r, "description", "tfacc line one\nline two"),
					tfresource.TestCheckResourceAttr(r, "criticality", "critical"),
					tfresource.TestCheckResourceAttrPair(r, "owner_team_id", "uptimeify_im_team.t", "id"),
					tfresource.TestCheckResourceAttr(r, "monitors.#", "1"),
					tfresource.TestCheckTypeSetElemNestedAttrs(r, "monitors.*", map[string]string{"monitor_type": "website", "monitor_id": monitorID}),
				),
			},
			{ResourceName: r, ImportState: true, ImportStateVerify: true},
			{ // a foreign or unknown monitor is the API's 404, the service keeps its monitors
				Config:      config(name+"-2", `monitors = [{ monitor_type = "dns", monitor_id = 2147483647 }]`),
				ExpectError: regexp.MustCompile(`imServiceMonitorNotFound`),
			},
			{ // the name is unique regardless of case
				Config: config(name+"-2", full) + fmt.Sprintf(`
resource "uptimeify_im_service" "dup" { name = %q }
`, strings.ToUpper(name+"-2")),
				ExpectError: regexp.MustCompile(`imServiceNameTaken`),
			},
			{ // drift: changed outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if _, err := accClient().UpdateIMService(context.Background(), serviceID(), map[string]any{"criticality": "low"}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             config(name+"-2", full),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // removed from the configuration: cleared in the API
				Config: config(name+"-2", ""),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					accSameInt(r, "id", &id),
					tfresource.TestCheckResourceAttr(r, "criticality", "medium"),
					tfresource.TestCheckNoResourceAttr(r, "description"),
					tfresource.TestCheckNoResourceAttr(r, "owner_team_id"),
					tfresource.TestCheckResourceAttr(r, "monitors.#", "0"),
				),
			},
			{ // drift: deleted outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					if err := accClient().DeleteIMService(context.Background(), serviceID()); err != nil {
						t.Fatal(err)
					}
				},
				Config:             config(name+"-2", ""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config(name+"-2", ""),
				Check:  accCaptureInt(r, "id", &id),
			},
		},
	})
}
