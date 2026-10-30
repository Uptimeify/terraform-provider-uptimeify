resource "uptimeify_report" "weekly" {
  name             = "Weekly fleet report"
  frequency        = "weekly"
  weekday          = 1
  send_hour        = 7
  format           = "email_pdf"
  recipient_emails = ["ops@example.com"]

  # The status pages section is on by default; leave it out of this report.
  include_status_pages = false
}
