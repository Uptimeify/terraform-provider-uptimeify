resource "uptimeify_status_page_maintenance" "db_upgrade" {
  title           = "Database upgrade"
  description     = "The API is read-only for about 30 minutes."
  scheduled_for   = "2026-11-03T22:00:00+01:00"
  scheduled_until = "2026-11-03T23:00:00+01:00"
  status_page_ids = [tonumber(uptimeify_status_page.main.id)]
  # Only these components show the maintenance; leave out for the whole page.
  component_ids = [uptimeify_status_page_component.api.component_id]

  reminder_minutes_before = 60
  notify_on_start         = true
}
