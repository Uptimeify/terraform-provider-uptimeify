package provider

import (
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccMonitoringLocations(t *testing.T) {
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
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
