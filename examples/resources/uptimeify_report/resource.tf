resource "uptimeify_report" "weekly" {
  name             = "Weekly fleet report"
  frequency        = "weekly"
  weekday          = 1
  send_hour        = 7
  format           = "email_pdf"
  recipient_emails = ["ops@example.com"]
}
