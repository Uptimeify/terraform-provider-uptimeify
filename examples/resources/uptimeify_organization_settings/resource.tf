# Only the attributes set here are managed; everything else stays as it is.
resource "uptimeify_organization_settings" "this" {
  company_name = "Example GmbH"
  street       = "Hauptstrasse 1"
  postal_code  = "10115"
  city         = "Berlin"
  country      = "Germany"

  default_notification_channels = {
    email        = true
    sms          = false
    webhook      = false
    integrations = true
  }
}
