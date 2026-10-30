resource "uptimeify_package_config" "business" {
  package_type       = "business"
  display_name       = "Business"
  max_urls           = 50
  max_status_pages   = 3 # unset = unlimited
  allow_self_service = true
}
