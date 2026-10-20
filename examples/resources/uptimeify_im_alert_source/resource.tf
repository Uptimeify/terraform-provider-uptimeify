variable "grafana_hmac_secret" {
  type      = string
  sensitive = true
}

# Grafana webhook with the preset mapping, HMAC signature and a weekly quiet window.
resource "uptimeify_im_alert_source" "grafana" {
  team_id = 7
  name    = "Grafana production"
  type    = "grafana"

  # Write-only: never stored in state. Bump the version to send a new secret.
  hmac_secret_wo         = var.grafana_hmac_secret
  hmac_secret_wo_version = 1

  maintenance_windows = [{ days_of_week = [7], from = "02:00", to = "04:00", timezone = "Europe/Berlin" }]
}

# Custom webhook with a v2 attribute mapping and bearer auth.
resource "uptimeify_im_alert_source" "custom" {
  team_id        = 7
  name           = "Billing jobs"
  type           = "custom"
  bearer_enabled = true
  # Change to rotate the bearer token or the ingest token.
  bearer_token_rotation = "2026-10"
  ingest_token_rotation = "2026-10"

  payload_mapping = {
    default_severity = "sev3"
    attributes = [
      { key = "title", standard = true, steps = [{ kind = "extract", type = "path", expr = "job.name" }] },
      { key = "severity", standard = true, steps = [
        { kind = "extract", type = "path", expr = "level" },
        { kind = "valueMap", entries = [{ from = "fatal", to = "sev1" }, { from = "error", to = "sev2" }], fallback = "sev3" },
      ] },
    ]
  }
}

output "billing_webhook_url" {
  value     = uptimeify_im_alert_source.custom.ingest_url
  sensitive = true
}
