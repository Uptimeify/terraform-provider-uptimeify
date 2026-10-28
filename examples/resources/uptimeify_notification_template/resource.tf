# One per organization: the chain new users start with. Existing users keep theirs.
resource "uptimeify_notification_template" "this" {
  rules = [
    { urgency = "high", delay_minutes = 0, channel = "push" },
    { urgency = "high", delay_minutes = 10, channel = "sms" },
    { urgency = "low", delay_minutes = 0, channel = "email" },
  ]
}
