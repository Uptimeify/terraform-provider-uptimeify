resource "uptimeify_im_escalation_tier" "first" {
  team_id                       = uptimeify_im_team.platform.id
  tier_order                    = 1
  member_user_ids               = [uptimeify_im_team_member.alice.user_id]
  auto_escalation_after_minutes = 10
  auto_escalation_severities    = ["critical", "warning"]

  # Escalate automatically only on weekdays, office hours.
  auto_escalation_time_filters = [
    { from = "08:00", until = "18:00", days = [1, 2, 3, 4, 5] },
  ]
}

resource "uptimeify_im_escalation_tier" "second" {
  team_id         = uptimeify_im_team.platform.id
  tier_order      = 2
  member_user_ids = [uptimeify_im_team_member.bob.user_id]
}
