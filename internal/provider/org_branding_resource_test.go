package provider

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// writePNG writes a 1x1 PNG of color c to dir/name.
func writePNG(t *testing.T, dir, name string, c color.Color) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, c)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Acceptance: changes the branding of a second organization (UPTIMEIFY_ACC_TOKEN_BRANDING, else the
// SMTP test's UPTIMEIFY_ACC_TOKEN_SMTP), never the main test organization's real branding. Skips
// when that organization already has a product name, colors or links. The uploaded test logos stay
// after the run (the API cannot delete images).
func TestAccOrganizationBranding(t *testing.T) {
	orgAccStart(t)
	token := os.Getenv("UPTIMEIFY_ACC_TOKEN_BRANDING")
	if token == "" {
		token = os.Getenv("UPTIMEIFY_ACC_TOKEN_SMTP")
	}
	if token == "" {
		t.Skip("UPTIMEIFY_ACC_TOKEN_BRANDING (or UPTIMEIFY_ACC_TOKEN_SMTP) not set: needs an org-admin token of a second organization")
	}
	bc := func() *client.Client { return client.New(os.Getenv("UPTIMEIFY_ENDPOINT"), token, "test") }
	isEmpty := func(b *client.Branding) bool {
		return b.ProductName == nil && b.ThemePrimary == nil && b.ThemeSecondary == nil && b.ImprintURL == nil &&
			b.PrivacyURL == nil && b.TermsURL == nil && !b.HideLogos && !b.HideProductName
	}
	cur, err := bc().GetBranding(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !isEmpty(cur) {
		t.Skip("organization already has its own branding; not touching it")
	}
	dir := t.TempDir()
	red := writePNG(t, dir, "red.png", color.RGBA{255, 0, 0, 255})
	blue := writePNG(t, dir, "blue.png", color.RGBA{0, 0, 255, 255})
	const r = "uptimeify_organization_branding.b"
	cfg := func(name, logo, extra string) string {
		return fmt.Sprintf(`
provider "uptimeify" {
  api_token = %q
}

resource "uptimeify_organization_branding" "b" {
  product_name    = %q
  theme_primary   = "#43B1AE"
  theme_secondary = "slate"
  imprint_url     = "https://tfacc.example/imprint"
  logo_light_file = %q
  %s
}
`, token, name, logo, extra)
	}
	var firstSum string
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			b, err := bc().GetBranding(context.Background())
			if err != nil {
				return err
			}
			if !isEmpty(b) {
				return fmt.Errorf("branding not reset after destroy: %+v", b)
			}
			// DELETE answers 404 only when nothing is uploaded (a probe that would also clean up).
			if err := bc().DeleteBrandingAsset(context.Background(), "logoLight"); !client.IsNotFound(err) {
				return fmt.Errorf("light logo still uploaded after destroy (err=%v)", err)
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{
				Config: cfg("tfacc-brand", red, ""),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(r, "product_name", "tfacc-brand"),
					tfresource.TestCheckResourceAttr(r, "theme_secondary", "slate"),
					tfresource.TestCheckResourceAttr(r, "hide_logos", "false"),
					tfresource.TestCheckNoResourceAttr(r, "logo_dark_sha256"),
					func(s *terraform.State) error {
						firstSum = s.RootModule().Resources[r].Primary.Attributes["logo_light_sha256"]
						if len(firstSum) != 64 {
							return fmt.Errorf("logo_light_sha256 %q", firstSum)
						}
						return nil
					},
				),
			},
			{ // new image content, a dark logo and a flag
				Config: cfg("tfacc-brand-2", blue, fmt.Sprintf("logo_dark_file = %q\n  hide_product_name = true", red)),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(r, "product_name", "tfacc-brand-2"),
					tfresource.TestCheckResourceAttr(r, "hide_product_name", "true"),
					func(s *terraform.State) error {
						a := s.RootModule().Resources[r].Primary.Attributes
						if a["logo_light_sha256"] == firstSum || a["logo_dark_sha256"] != firstSum {
							return fmt.Errorf("hashes not updated: light=%s dark=%s first=%s", a["logo_light_sha256"], a["logo_dark_sha256"], firstSum)
						}
						return nil
					},
				),
			},
			{
				ResourceName: r, ImportState: true, ImportStateId: "branding", ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"logo_light_file", "logo_light_sha256", "logo_dark_file", "logo_dark_sha256"},
			},
			{ // drift: renamed outside Terraform
				PreConfig: func() {
					if err := bc().UpdateBranding(context.Background(), map[string]any{"productName": "tfacc-drift"}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             cfg("tfacc-brand-2", blue, fmt.Sprintf("logo_dark_file = %q\n  hide_product_name = true", red)),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: cfg("tfacc-brand-2", blue, fmt.Sprintf("logo_dark_file = %q\n  hide_product_name = true", red))},
			{ // removing logo_dark_file deletes the uploaded dark logo
				Config: cfg("tfacc-brand-2", blue, "hide_product_name = true"),
				Check: func(*terraform.State) error {
					if err := bc().DeleteBrandingAsset(context.Background(), "logoDark"); !client.IsNotFound(err) {
						return fmt.Errorf("dark logo still uploaded (err=%v)", err)
					}
					return nil
				},
			},
		},
	})
}
