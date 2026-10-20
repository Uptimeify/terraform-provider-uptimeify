# Weekly rotation, handover Monday 09:00 Berlin time.
resource "uptimeify_im_schedule" "weekly" {
  tier_id  = uptimeify_im_escalation_tier.first.tier_id
  name     = "Weekly on-call"
  timezone = "Europe/Berlin"

  rotation_mode         = "explicit"
  rotation_repeats      = "weekly"
  starts_on_day_of_week = 1
  starts_on_time        = "09:00"

  rotations = [
    { members = [uptimeify_im_team_member.alice.user_id] },
    { members = [uptimeify_im_team_member.bob.user_id] },
  ]
}
