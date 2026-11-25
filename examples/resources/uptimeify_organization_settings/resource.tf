# Only the attributes set here are managed; everything else stays as it is.
resource "uptimeify_organization_settings" "this" {
  company_name = "Example GmbH"
  street       = "Hauptstrasse 1"
  postal_code  = "10115"
  city         = "Berlin"
  country      = "Germany"

  # Notices of changes to the terms, the DPA and the privacy policy go here instead of to every admin.
  contract_email = "legal@example.com"

  default_notification_channels = {
    email        = true
    sms          = false
    webhook      = false
    integrations = true
  }

  # Per check type: which channels alert for new customers.
  default_notification_rules = {
    http_status = { email = true, sms = true, webhook = false, integrations = true }
    ssl_expiry  = { email = true, sms = false, webhook = false, integrations = false }
  }
}
