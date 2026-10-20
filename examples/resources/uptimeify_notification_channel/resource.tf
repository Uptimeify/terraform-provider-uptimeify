resource "uptimeify_notification_channel" "ops_mail" {
  type   = "email"
  name   = "Ops mail"
  config = jsonencode({ email = "ops@example.com" })
}

# Secrets go into the write-only attribute; bump the version to send a new one.
variable "slack_webhook_url" {
  type      = string
  sensitive = true
}

resource "uptimeify_notification_channel" "slack" {
  type                     = "slack"
  category                 = "integration"
  name                     = "Slack #alerts"
  customer_id              = tonumber(uptimeify_customer.shop.id)
  config                   = jsonencode({})
  secret_config_wo         = jsonencode({ webhookUrl = var.slack_webhook_url })
  secret_config_wo_version = 1
}
