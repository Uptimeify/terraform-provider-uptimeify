variable "escalation_token" {
  type      = string
  sensitive = true
}

resource "uptimeify_escalation_config" "this" {
  webhook_url           = "https://hooks.example.com/uptimeify"
  webhook_method        = "POST"
  webhook_timeout       = 10
  expected_status_codes = "200,202,204"
  is_active             = true

  webhook_headers_wo         = { "X-Api-Token" = var.escalation_token }
  webhook_headers_wo_version = 1
}
