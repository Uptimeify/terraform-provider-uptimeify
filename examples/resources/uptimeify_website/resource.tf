data "uptimeify_customer" "shop" {
  name = "Example Shop GmbH"
}

variable "api_key" {
  type      = string
  sensitive = true
}

# HTTP check with request headers, body, content checks and a connect target.
resource "uptimeify_website" "api" {
  customer_id     = tonumber(data.uptimeify_customer.shop.id)
  name            = "Shop API health"
  url             = "https://shop.example.com/api/health"
  http_method     = "POST"
  check_interval  = 5
  timeout_seconds = 10

  expected_status_codes = "200"
  check_keyword_enabled = true
  search_term           = "\"status\":\"ok\""

  allowed_check_country_codes = ["DE", "NL"]

  # Check the origin directly; SNI and Host header stay shop.example.com.
  connect_host = "origin.example.com"
  connect_port = 8443

  # Write-only: never stored in state, never returned by the API.
  # Bump the version to send changed values.
  custom_headers_wo = [
    { name = "X-Api-Key", value = var.api_key },
    { name = "Accept", value = "application/json" },
  ]
  custom_headers_wo_version = 1
  request_body_wo           = jsonencode({ probe = true })
  request_body_wo_version   = 1
}

# Heartbeat: the monitored job calls the ping URL with heartbeat_token.
resource "uptimeify_website" "nightly_backup" {
  customer_id                    = tonumber(data.uptimeify_customer.shop.id)
  name                           = "Nightly backup"
  monitoring_type                = "heartbeat"
  check_interval                 = 1440
  heartbeat_grace_period_minutes = 60

  # Change to any new value to mint a new token. The old token stops working
  # immediately, so roll out the new ping URL in the same change.
  heartbeat_token_rotation = "2026-10"
}
