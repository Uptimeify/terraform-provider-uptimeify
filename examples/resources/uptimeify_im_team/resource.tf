resource "uptimeify_im_team" "platform" {
  name     = "Platform"
  timezone = "Europe/Berlin"

  escalation_repeats            = 1
  escalation_repeats_stop_mode  = "acknowledged"
  engagement_report_enabled     = true
  engagement_report_day_of_week = 1
  engagement_report_time        = "08:30"
}
