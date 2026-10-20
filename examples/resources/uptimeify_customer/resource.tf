resource "uptimeify_customer" "shop" {
  name               = "Example Shop GmbH"
  package_type       = "business" # or package_id = 3
  notification_email = "ops@example-shop.com"
  custom_fields      = jsonencode({ contract = "A-1" })

  notification_targets = { email = "customer", sms = "organization" }
  allow_self_service   = true
}
