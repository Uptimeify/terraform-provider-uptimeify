package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Unit: create sends every known limit (unset max_status_pages and notes as null), update only
// the changed ones; unknown computed values are never sent.
func TestPackageConfigPatch(t *testing.T) {
	plan := packageConfigModel{PackageType: types.StringValue("gold"), DisplayName: types.StringUnknown(),
		MaxURLs: types.Int64Value(10), MaxStatusPages: types.Int64Null(), AllowSelfService: types.BoolUnknown(),
		MaxSelfServiceURLs: types.Int64Unknown(), Notes: types.StringUnknown()}
	got := packageConfigPatch(&plan, nil)
	if len(got) != 2 || got["maxUrls"] != int64(10) || got["maxStatusPages"] != (*int64)(nil) {
		t.Fatalf("create patch: %v", got)
	}
	state := plan
	state.Notes, plan.Notes = types.StringNull(), types.StringNull()
	state.DisplayName, state.AllowSelfService, state.MaxSelfServiceURLs = types.StringValue("Gold"), types.BoolValue(false), types.Int64Value(0)
	plan.DisplayName, plan.AllowSelfService, plan.MaxSelfServiceURLs = state.DisplayName, state.AllowSelfService, state.MaxSelfServiceURLs
	plan.MaxStatusPages = types.Int64Value(3)
	if got := packageConfigPatch(&plan, &state); len(got) != 1 || *got["maxStatusPages"].(*int64) != 3 {
		t.Fatalf("update patch: %v", got)
	}
}

// ---- acceptance ------------------------------------------------------------------------------

func packageConfigGone(pt string) func(*terraform.State) error {
	return func(*terraform.State) error {
		pcs, err := accClient().ListPackageConfigs(context.Background())
		if err != nil {
			return err
		}
		for _, p := range pcs {
			if p.PackageType == pt {
				return fmt.Errorf("package %s still exists", pt)
			}
		}
		return nil
	}
}

func TestAccPackageConfig(t *testing.T) {
	spAccSkip(t)
	pt := fmt.Sprintf("tfacc-pkg-%d", time.Now().UnixNano()%1_000_000)
	const r = "uptimeify_package_config.p"
	cfg := func(extra string) string {
		return fmt.Sprintf(`
resource "uptimeify_package_config" "p" {
  package_type = %q
  display_name = "TF Acc"
  max_urls     = 10
  %s
}
data "uptimeify_package_configs" "all" { depends_on = [uptimeify_package_config.p] }
`, pt, extra)
	}
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy:             packageConfigGone(pt),
		Steps: []tfresource.TestStep{
			{
				Config: cfg(`max_status_pages = 2
  allow_self_service = true
  notes              = "tfacc"`),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(r, "id", pt),
					tfresource.TestCheckResourceAttr(r, "max_urls", "10"),
					tfresource.TestCheckResourceAttr(r, "max_status_pages", "2"),
					tfresource.TestCheckResourceAttr(r, "allow_self_service", "true"),
					tfresource.TestCheckResourceAttr(r, "notes", "tfacc"),
					tfresource.TestCheckTypeSetElemNestedAttrs("data.uptimeify_package_configs.all", "package_configs.*",
						map[string]string{"package_type": pt, "max_status_pages": "2"}),
				),
			},
			{ // unset: max_status_pages unlimited; notes and allow_self_service kept
				Config: cfg(""),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckNoResourceAttr(r, "max_status_pages"),
					tfresource.TestCheckResourceAttr(r, "notes", "tfacc"),
					tfresource.TestCheckResourceAttr(r, "allow_self_service", "true"),
				),
			},
			{ResourceName: r, ImportState: true, ImportStateId: pt, ImportStateVerify: true},
		},
	})
}
