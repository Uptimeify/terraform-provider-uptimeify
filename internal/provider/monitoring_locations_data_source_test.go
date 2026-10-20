package provider

import (
	"os"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccMonitoringLocations(t *testing.T) {
	tfresource.Test(t, tfresource.TestCase{
		PreCheck: func() {
			if os.Getenv("UPTIMEIFY_API_TOKEN") == "" {
				t.Fatal("UPTIMEIFY_API_TOKEN must be set for acceptance tests")
			}
		},
		ProtoV6ProviderFactories: testAccProviders,
		Steps: []tfresource.TestStep{{
			Config: `
terraform {
  required_providers {
    uptimeify = { source = "uptimeify/uptimeify" }
  }
}

data "uptimeify_monitoring_locations" "all" {}
`,
			Check: tfresource.ComposeAggregateTestCheckFunc(
				tfresource.TestCheckResourceAttr("data.uptimeify_monitoring_locations.all", "id", "monitoring-locations"),
				tfresource.TestCheckResourceAttrSet("data.uptimeify_monitoring_locations.all", "country_codes.#"),
			),
		}},
	})
}
