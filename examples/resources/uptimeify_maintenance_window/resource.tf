# Weekly Sunday-night window for every monitor tagged "production".
resource "uptimeify_maintenance_window" "patch_night" {
  name        = "Patch night"
  description = "OS updates"
  start_time  = "2030-01-06T02:00:00+01:00"
  end_time    = "2030-01-06T04:00:00+01:00"
  timezone    = "Europe/Berlin"
  tag_ids     = [tonumber(uptimeify_tag.production.id)]

  recurrence_pattern = {
    frequency    = "weekly"
    days_of_week = [0]
  }
}

# One-off window for two explicit monitors.
resource "uptimeify_maintenance_window" "migration" {
  name       = "DB migration"
  start_time = "2030-03-01T22:00:00Z"
  end_time   = "2030-03-02T01:00:00Z"
  targets = [
    { type = "website", id = 12 },
    { type = "dns", id = 7 },
  ]
}

# Every monitor of one customer.
resource "uptimeify_maintenance_window" "customer" {
  name        = "Customer move"
  start_time  = "2030-04-01T00:00:00Z"
  end_time    = "2030-04-01T06:00:00Z"
  customer_id = 3
}
