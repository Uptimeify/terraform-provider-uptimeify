# Critical database alerts from Grafana go to the DBA team as sev1.
resource "uptimeify_im_routing_rule" "db" {
  team_id  = 9
  priority = 10
  match = {
    source_ids    = [tonumber(uptimeify_im_alert_source.grafana.id)]
    severity      = ["sev1", "sev2"]
    tags          = ["db"]
    field_matches = { env = "prod" }
  }
  severity_override = "sev1"
}

# Catch-all at the end.
resource "uptimeify_im_routing_rule" "default" {
  team_id  = 7
  priority = 1000
}
