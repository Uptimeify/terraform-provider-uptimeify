resource "uptimeify_organization_branding" "this" {
  product_name    = "ACME Monitoring"
  theme_primary   = "#43B1AE"
  theme_secondary = "slate"
  imprint_url     = "https://acme.example/imprint"
  privacy_url     = "https://acme.example/privacy"

  # Local files; a changed file is uploaded again on the next apply.
  logo_light_file    = "${path.module}/branding/logo-light.png"
  logo_dark_file     = "${path.module}/branding/logo-dark.png"
  favicon_light_file = "${path.module}/branding/favicon.ico"
}
