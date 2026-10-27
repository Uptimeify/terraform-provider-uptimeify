package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// Unit: there is no single GET; a component missing from the page's list reads as 404.
func TestStatusPageComponentReadByList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status-pages/4/components" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"id":9,"statusPageId":4,"name":"API","sourceKind":"manual","manualStatus":"degraded"}],"total":1}`))
	}))
	defer srv.Close()
	c := client.New(srv.URL, "wsm_test", "test")
	got, err := c.GetStatusPageComponent(context.Background(), 4, 9)
	if err != nil || got.Name != "API" || *got.ManualStatus != "degraded" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := c.GetStatusPageComponent(context.Background(), 4, 10); !client.IsNotFound(err) {
		t.Fatalf("want 404 for a missing component, got %v", err)
	}
}

// Unit: written custom_css stays in state while the server's stored form is unchanged.
func TestStatusPageDesignKeepCSS(t *testing.T) {
	m := statusPageDesignModel{}
	m.fromAPI(&client.StatusPageDesign{CustomCSS: ".a{color:red}"})
	m.keepCSS(types.StringValue(".a { color: red; }"), types.StringUnknown()) // after a write
	if m.CustomCSS.ValueString() != ".a { color: red; }" || m.CustomCSSStored.ValueString() != ".a{color:red}" {
		t.Fatalf("after write: %v / %v", m.CustomCSS, m.CustomCSSStored)
	}
	written, last := m.CustomCSS, m.CustomCSSStored
	m.fromAPI(&client.StatusPageDesign{CustomCSS: ".a{color:red}"}) // read, unchanged
	m.keepCSS(written, last)
	if m.CustomCSS.ValueString() != ".a { color: red; }" {
		t.Fatalf("unchanged read lost the written text: %v", m.CustomCSS)
	}
	m.fromAPI(&client.StatusPageDesign{CustomCSS: ".b{color:blue}"}) // read, changed elsewhere
	m.keepCSS(written, last)
	if m.CustomCSS.ValueString() != ".b{color:blue}" {
		t.Fatalf("drift not shown: %v", m.CustomCSS)
	}
}

// Acceptance: groups and components on one page: create, update in place, import, drift, destroy.
// manual always; monitor with UPTIMEIFY_ACC_PAGE_WEBSITE_ID (a website of the test customer, so
// the test needs no monitor quota); dependency with UPTIMEIFY_ACC_DEPENDENCY_SLUG.
func TestAccStatusPageComponents(t *testing.T) {
	spAccSkip(t)
	name := fmt.Sprintf("tfacc-spc-%d", time.Now().UnixNano()%1_000_000)
	dep := os.Getenv("UPTIMEIFY_ACC_DEPENDENCY_SLUG")
	website := os.Getenv("UPTIMEIFY_ACC_PAGE_WEBSITE_ID")
	const g, man, mon = "uptimeify_status_page_component_group.g", "uptimeify_status_page_component.manual", "uptimeify_status_page_component.monitor"
	var pageID, groupID, manualID string
	cfg := func(groupName, manual string, collapsed bool) string {
		c := statusPageConfig(t, name, name, "include_customer_monitors = false") + fmt.Sprintf(`
resource "uptimeify_status_page_component_group" "g" {
  status_page_id = tonumber(uptimeify_status_page.p.id)
  name           = %[2]q
  collapsed      = %[4]t
}

resource "uptimeify_status_page_component" "manual" {
  status_page_id = tonumber(uptimeify_status_page.p.id)
  source_kind    = "manual"
  %[3]s
}
`, name, groupName, manual, collapsed)
		if website != "" {
			c += fmt.Sprintf(`
resource "uptimeify_status_page_component" "monitor" {
  status_page_id = tonumber(uptimeify_status_page.p.id)
  source_kind    = "monitor"
  monitor_type   = "http"
  monitor_id     = %s
  group_id       = uptimeify_status_page_component_group.g.group_id
}
`, website)
		}
		if dep != "" {
			c += fmt.Sprintf(`
resource "uptimeify_status_page_component" "dep" {
  status_page_id  = tonumber(uptimeify_status_page.p.id)
  source_kind     = "dependency"
  dependency_slug = %q
}
`, dep)
		}
		return c
	}
	step1 := cfg("Core", `name = "Support"`, false)
	step2 := cfg("Core services", `name = "Phone support"
  description   = "Mon to Fri"
  manual_status = "partial_outage"
  group_id      = uptimeify_status_page_component_group.g.group_id
  sort          = 7
  show_uptime   = false`, true)
	monChecks := func(checks ...tfresource.TestCheckFunc) tfresource.TestCheckFunc {
		if website == "" {
			return func(*terraform.State) error { return nil }
		}
		return tfresource.ComposeAggregateTestCheckFunc(checks...)
	}
	depChecks := func() tfresource.TestCheckFunc {
		if dep == "" {
			return func(*terraform.State) error { return nil }
		}
		return tfresource.ComposeAggregateTestCheckFunc(
			tfresource.TestCheckResourceAttr("uptimeify_status_page_component.dep", "dependency_slug", dep),
			tfresource.TestCheckResourceAttrSet("uptimeify_status_page_component.dep", "name"),
			tfresource.TestCheckNoResourceAttr("uptimeify_status_page_component.dep", "manual_status"),
		)
	}

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy:             statusPageGone(&pageID),
		Steps: []tfresource.TestStep{
			{
				Config: step1,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSaveID("uptimeify_status_page.p", &pageID),
					spSaveID(g, &groupID),
					spSaveID(man, &manualID),
					tfresource.TestCheckResourceAttr("uptimeify_status_page.p", "include_customer_monitors", "false"),
					tfresource.TestCheckResourceAttr(g, "name", "Core"),
					tfresource.TestCheckResourceAttr(g, "collapsed", "false"),
					tfresource.TestCheckResourceAttr(man, "manual_status", "operational"),
					tfresource.TestCheckResourceAttr(man, "show_uptime", "true"),
					tfresource.TestCheckNoResourceAttr(man, "group_id"),
					monChecks(
						tfresource.TestCheckResourceAttrSet(mon, "name"),
						tfresource.TestCheckNoResourceAttr(mon, "manual_status"),
						tfresource.TestCheckResourceAttrPair(mon, "group_id", g, "group_id"),
					),
					depChecks(),
				),
			},
			{ // update in place
				Config: step2,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSameID(g, &groupID),
					spSameID(man, &manualID),
					tfresource.TestCheckResourceAttr(g, "name", "Core services"),
					tfresource.TestCheckResourceAttr(g, "collapsed", "true"),
					tfresource.TestCheckResourceAttr(man, "name", "Phone support"),
					tfresource.TestCheckResourceAttr(man, "description", "Mon to Fri"),
					tfresource.TestCheckResourceAttr(man, "manual_status", "partial_outage"),
					tfresource.TestCheckResourceAttr(man, "sort", "7"),
					tfresource.TestCheckResourceAttr(man, "show_uptime", "false"),
					tfresource.TestCheckResourceAttrPair(man, "group_id", g, "group_id"),
				),
			},
			{ResourceName: g, ImportState: true, ImportStateVerify: true},
			{ResourceName: man, ImportState: true, ImportStateVerify: true},
			{ // drift: status changed and group deleted outside Terraform
				PreConfig: func() {
					accPreCheck(t)
					p, _ := strconv.ParseInt(pageID, 10, 64)
					_, gid, _ := parseTwoIDs(groupID, "")
					_, cid, _ := parseTwoIDs(manualID, "")
					if _, err := accClient().UpdateStatusPageComponent(context.Background(), p, cid, map[string]any{"manualStatus": "major_outage"}); err != nil {
						t.Fatal(err)
					}
					if err := accClient().DeleteStatusPageComponentGroup(context.Background(), p, gid); err != nil {
						t.Fatal(err)
					}
				},
				Config:             step2,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // apply repairs both; the group comes back with a new id
				Config: step2,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					spSameID(man, &manualID),
					tfresource.TestCheckResourceAttr(man, "manual_status", "partial_outage"),
					tfresource.TestCheckResourceAttrPair(man, "group_id", g, "group_id"),
					monChecks(tfresource.TestCheckResourceAttrPair(mon, "group_id", g, "group_id")),
				),
			},
			{ // unsetting group_id and description clears them on the server
				Config: cfg("Core services", `name = "Phone support"`, true),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckNoResourceAttr(man, "group_id"),
					tfresource.TestCheckNoResourceAttr(man, "description"),
					func(*terraform.State) error {
						p, _ := strconv.ParseInt(pageID, 10, 64)
						_, cid, _ := parseTwoIDs(manualID, "")
						c, err := accClient().GetStatusPageComponent(context.Background(), p, cid)
						if err != nil {
							return err
						}
						if c.GroupID != nil || c.Description != nil {
							return fmt.Errorf("not cleared on the server: %+v", c)
						}
						return nil
					},
				),
			},
		},
	})
}
