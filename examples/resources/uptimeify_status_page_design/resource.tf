resource "uptimeify_status_page_design" "acme" {
  status_page_id  = tonumber(uptimeify_status_page.acme.id)
  layout          = "statusboard"
  color_scheme    = "auto"
  accent_color    = "#0f766e"
  custom_title    = "ACME Status"
  show_powered_by = false
  history_days    = 90
  incident_limit  = 10
}
