# An org admin sets up how a responder is paged. SMS and voice steps wait until the
# responder confirms the number (they are asked to on their next sign-in).
data "uptimeify_user" "rita" {
  email = "rita@example.com"
}

resource "uptimeify_user_notification_settings" "rita" {
  user_id      = data.uptimeify_user.rita.id
  phone_number = "+491701234567"

  rules = [
    { urgency = "high", delay_minutes = 0, channel = "push" },
    { urgency = "high", delay_minutes = 5, channel = "sms" },
    { urgency = "high", delay_minutes = 15, channel = "voice" },
    { urgency = "low", delay_minutes = 0, channel = "email" },
  ]
}
