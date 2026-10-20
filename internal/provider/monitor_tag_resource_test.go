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
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// fakeMonitorTags is an in-memory /api/monitor-tags with the 5-tag limit.
func fakeMonitorTags(t *testing.T, assigned map[int64]bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/monitor-tags" {
			t.Errorf("path %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			q := r.URL.Query()
			if q.Get("monitorType") != "website" || q.Get("monitorId") != "12" || q.Get("envelope") != "items" {
				t.Errorf("query %s", r.URL.RawQuery)
			}
			items := []string{}
			for id := range assigned {
				items = append(items, fmt.Sprintf(`{"id":%d,"name":"t%d","color":"red"}`, id, id))
			}
			fmt.Fprintf(w, `{"items":[%s],"total":%d,"page":1,"perPage":%d}`, strings.Join(items, ","), len(items), len(items))
		case http.MethodPost, http.MethodDelete:
			var ref client.MonitorTagRef
			if err := json.NewDecoder(r.Body).Decode(&ref); err != nil || ref.MonitorType != "website" || ref.MonitorID != 12 {
				t.Errorf("body %+v %v", ref, err)
			}
			if r.Method == http.MethodDelete {
				delete(assigned, ref.TagID)
				_, _ = w.Write([]byte(`{"success":true}`))
				return
			}
			if !assigned[ref.TagID] && len(assigned) >= client.MaxTagsPerMonitor {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"statusCode":422,"statusMessage":"A monitor can have at most 5 tags","data":{"code":"tagLimitReached","max":5}}`))
				return
			}
			assigned[ref.TagID] = true
			fmt.Fprintf(w, `{"id":1,"tagId":%d,"monitorType":"website","monitorId":12,"organizationId":2,"createdAt":"x"}`, ref.TagID)
		}
	}))
}

func monitorTagHarness(t *testing.T, srv *httptest.Server) (*monitorTagResource, resource.SchemaResponse) {
	r := &monitorTagResource{client: client.New(srv.URL, "wsm_test", "test")}
	var sch resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &sch)
	return r, sch
}

func monitorTagPlan(t *testing.T, sch resource.SchemaResponse, tagID int64) tfsdk.Plan {
	plan := tfsdk.Plan{Schema: sch.Schema}
	if d := plan.Set(context.Background(), &monitorTagModel{ID: types.StringUnknown(), MonitorType: types.StringValue("website"),
		MonitorID: types.Int64Value(12), TagID: types.Int64Value(tagID)}); d.HasError() {
		t.Fatal(d)
	}
	return plan
}

func TestMonitorTagCreateReadDelete(t *testing.T) {
	assigned := map[int64]bool{}
	srv := fakeMonitorTags(t, assigned)
	defer srv.Close()
	ctx := context.Background()
	r, sch := monitorTagHarness(t, srv)

	cresp := resource.CreateResponse{State: tfsdk.State{Schema: sch.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: monitorTagPlan(t, sch, 34)}, &cresp)
	if cresp.Diagnostics.HasError() || !assigned[34] {
		t.Fatal(cresp.Diagnostics)
	}
	var m monitorTagModel
	cresp.State.Get(ctx, &m)
	if m.ID.ValueString() != "website/12/34" {
		t.Fatalf("id %s", m.ID)
	}

	rresp := resource.ReadResponse{State: cresp.State}
	r.Read(ctx, resource.ReadRequest{State: cresp.State}, &rresp)
	if rresp.Diagnostics.HasError() || rresp.State.Raw.IsNull() {
		t.Fatalf("assigned tag lost: %v", rresp.Diagnostics)
	}

	delete(assigned, 34) // removed outside Terraform
	rresp = resource.ReadResponse{State: cresp.State}
	r.Read(ctx, resource.ReadRequest{State: cresp.State}, &rresp)
	if !rresp.State.Raw.IsNull() {
		t.Fatal("unassigned tag still in state")
	}

	assigned[34] = true
	dresp := resource.DeleteResponse{State: cresp.State}
	r.Delete(ctx, resource.DeleteRequest{State: cresp.State}, &dresp)
	if dresp.Diagnostics.HasError() || assigned[34] {
		t.Fatal(dresp.Diagnostics)
	}
}

func TestMonitorTagLimitIsClearError(t *testing.T) {
	assigned := map[int64]bool{1: true, 2: true, 3: true, 4: true, 5: true}
	srv := fakeMonitorTags(t, assigned)
	defer srv.Close()
	r, sch := monitorTagHarness(t, srv)

	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch.Schema}}
	r.Create(context.Background(), resource.CreateRequest{Plan: monitorTagPlan(t, sch, 6)}, &resp)
	if !resp.Diagnostics.HasError() || resp.Diagnostics[0].Summary() != "Tag limit reached" ||
		!strings.Contains(resp.Diagnostics[0].Detail(), "already carries 5 tags") {
		t.Fatalf("diags %v", resp.Diagnostics)
	}
}

func TestMonitorTagImportID(t *testing.T) {
	r, sch := monitorTagHarness(t, httptest.NewServer(http.NotFoundHandler()))
	ctx := context.Background()
	empty := tftypes.NewValue(sch.Schema.Type().TerraformType(ctx), nil)
	for id, ok := range map[string]bool{"website/12/34": true, "dns/1/2": true, "website/12": false, "nope/1/2": false, "website/x/2": false, "website/0/2": false} {
		resp := resource.ImportStateResponse{State: tfsdk.State{Schema: sch.Schema, Raw: empty}}
		r.ImportState(ctx, resource.ImportStateRequest{ID: id}, &resp)
		if resp.Diagnostics.HasError() == ok {
			t.Errorf("%s: diags %v", id, resp.Diagnostics)
			continue
		}
		if ok {
			var m monitorTagModel
			resp.State.Get(ctx, &m)
			if monitorTagID(m.ref()) != id {
				t.Errorf("%s imported as %+v", id, m)
			}
		}
	}
}

// Acceptance: needs a website the token can see, UPTIMEIFY_ACC_MONITOR_ID = its numeric id.
func TestAccMonitorTag(t *testing.T) {
	monitorID := os.Getenv("UPTIMEIFY_ACC_MONITOR_ID")
	if os.Getenv("TF_ACC") != "" && monitorID == "" {
		t.Skip("UPTIMEIFY_ACC_MONITOR_ID not set: the monitor-tag test needs the numeric id of an existing website")
	}
	tagName := fmt.Sprintf("tfacc-mt-%d", time.Now().Unix()%1_000_000)
	var tagID string
	mid, _ := strconv.ParseInt(monitorID, 10, 64)

	tfresource.Test(t, tfresource.TestCase{
		PreCheck: func() {
			if os.Getenv("UPTIMEIFY_API_TOKEN") == "" {
				t.Fatal("UPTIMEIFY_API_TOKEN must be set for acceptance tests")
			}
		},
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			tags, err := accClient().ListMonitorTags(context.Background(), "website", mid)
			if err != nil {
				return err
			}
			for _, tg := range tags {
				if strconv.FormatInt(tg.ID, 10) == tagID {
					return fmt.Errorf("tag %s still on website %d", tagID, mid)
				}
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: monitorTagConfig(tagName, monitorID),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_monitor_tag.mt", "monitor_type", "website"),
					tfresource.TestCheckResourceAttrPair("uptimeify_monitor_tag.mt", "tag_id", "uptimeify_tag.t", "id"),
					func(s *terraform.State) error {
						tagID = s.RootModule().Resources["uptimeify_tag.t"].Primary.ID
						want := "website/" + monitorID + "/" + tagID
						if got := s.RootModule().Resources["uptimeify_monitor_tag.mt"].Primary.ID; got != want {
							return fmt.Errorf("id %s, want %s", got, want)
						}
						return nil
					},
				),
			},
			{ // import verify
				ResourceName:      "uptimeify_monitor_tag.mt",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // drift: removed outside Terraform -> plan not empty
				PreConfig: func() {
					tid, _ := strconv.ParseInt(tagID, 10, 64)
					if err := accClient().RemoveMonitorTag(context.Background(), client.MonitorTagRef{MonitorType: "website", MonitorID: mid, TagID: tid}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             monitorTagConfig(tagName, monitorID),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // re-add, then destroy + CheckDestroy
				Config: monitorTagConfig(tagName, monitorID),
			},
		},
	})
}

func monitorTagConfig(tagName, monitorID string) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    uptimeify = { source = "uptimeify/uptimeify" }
  }
}

resource "uptimeify_tag" "t" {
  name = %q
}

resource "uptimeify_monitor_tag" "mt" {
  monitor_type = "website"
  monitor_id   = %s
  tag_id       = tonumber(uptimeify_tag.t.id)
}
`, tagName, monitorID)
}
