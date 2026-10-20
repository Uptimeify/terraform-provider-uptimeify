package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var testAccProviders = map[string]func() (tfprotov6.ProviderServer, error){
	"uptimeify": providerserver.NewProtocol6WithError(New("test")()),
}

// Unit: a tag deleted outside Terraform (404) leaves the state, so the next plan recreates it.
func TestTagReadNotFoundRemovesResource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"statusCode":404,"statusMessage":"Tag not found","data":{"code":"tagNotFound"}}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	r := &tagResource{client: client.New(srv.URL, "wsm_test", "test")}
	var sch resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sch)

	state := tfsdk.State{Schema: sch.Schema}
	if d := state.Set(ctx, &tagModel{ID: types.StringValue("42"), Name: types.StringValue("x"),
		Color: types.StringValue("red"), CustomerID: types.Int64Null()}); d.HasError() {
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

// Acceptance: needs TF_ACC=1, UPTIMEIFY_ENDPOINT, UPTIMEIFY_API_TOKEN and
// TF_ACC_PROVIDER_NAMESPACE=uptimeify (see scripts/acceptance.sh).
func TestAccTag(t *testing.T) {
	name := fmt.Sprintf("tfacc-%d", time.Now().Unix()%1_000_000)
	var tagID string

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			if _, err := accClient().GetTag(context.Background(), tagID); !client.IsNotFound(err) {
				return fmt.Errorf("tag %s still exists (err=%v)", tagID, err)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{ // create (default color)
				Config: tagConfig(name, ""),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_tag.t", "name", name),
					tfresource.TestCheckResourceAttr("uptimeify_tag.t", "color", DefaultTagColor),
					tfresource.TestCheckNoResourceAttr("uptimeify_tag.t", "customer_id"),
					tfresource.TestCheckResourceAttrSet("uptimeify_tag.t", "public_id"),
					func(s *terraform.State) error {
						tagID = s.RootModule().Resources["uptimeify_tag.t"].Primary.ID
						return nil
					},
				),
			},
			{ // update in place + data source by name and by id
				Config: tagConfig(name+"-2", "teal") + `
data "uptimeify_tag" "by_name" { name = uptimeify_tag.t.name }
data "uptimeify_tag" "by_id"   { id = uptimeify_tag.t.id }
`,
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr("uptimeify_tag.t", "name", name+"-2"),
					tfresource.TestCheckResourceAttr("uptimeify_tag.t", "color", "teal"),
					func(s *terraform.State) error {
						if got := s.RootModule().Resources["uptimeify_tag.t"].Primary.ID; got != tagID {
							return fmt.Errorf("tag was replaced: %s -> %s", tagID, got)
						}
						return nil
					},
					tfresource.TestCheckResourceAttrPair("data.uptimeify_tag.by_name", "public_id", "uptimeify_tag.t", "public_id"),
					tfresource.TestCheckResourceAttrPair("data.uptimeify_tag.by_id", "color", "uptimeify_tag.t", "color"),
				),
			},
			{ // import verify
				ResourceName:      "uptimeify_tag.t",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // drift: deleted outside Terraform -> plan not empty
				PreConfig: func() {
					if err := accClient().DeleteTag(context.Background(), tagID); err != nil {
						t.Fatal(err)
					}
				},
				Config:             tagConfig(name+"-2", "teal"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // recreate, then the framework destroys and CheckDestroy verifies
				Config: tagConfig(name+"-2", "teal"),
				Check: func(s *terraform.State) error {
					tagID = s.RootModule().Resources["uptimeify_tag.t"].Primary.ID
					return nil
				},
			},
		},
	})
}

// accPreCheck stops an acceptance run that would fall back to the production endpoint.
func accPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("UPTIMEIFY_ENDPOINT") == "" || os.Getenv("UPTIMEIFY_API_TOKEN") == "" {
		t.Fatal("UPTIMEIFY_ENDPOINT and UPTIMEIFY_API_TOKEN must be set for acceptance tests (no default endpoint in tests)")
	}
}

// accClient never falls back to DefaultEndpoint: without UPTIMEIFY_ENDPOINT every call fails locally.
func accClient() *client.Client {
	return client.New(os.Getenv("UPTIMEIFY_ENDPOINT"), os.Getenv("UPTIMEIFY_API_TOKEN"), "test")
}

func tagConfig(name, color string) string {
	c := ""
	if color != "" {
		c = fmt.Sprintf("color = %q", color)
	}
	// Host-relative source: resolves against the CLI's default registry, which
	// TF_ACC_PROVIDER_HOST must name (registry.opentofu.org for OpenTofu).
	return fmt.Sprintf(`
terraform {
  required_providers {
    uptimeify = { source = "uptimeify/uptimeify" }
  }
}

resource "uptimeify_tag" "t" {
  name = %q
  %s
}
`, name, c)
}
