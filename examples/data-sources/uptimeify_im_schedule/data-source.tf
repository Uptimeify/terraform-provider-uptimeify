data "uptimeify_im_team" "platform" {
  name = "Platform"
}

# Default schedules of different teams can share a name, team_id narrows the lookup.
data "uptimeify_im_schedule" "first_tier" {
  name    = "Tier 1 schedule"
  team_id = tonumber(data.uptimeify_im_team.platform.id)
}

# Take the first member of the looked-up schedule off call for the holidays.
resource "uptimeify_im_schedule_override" "holiday" {
  schedule_id = data.uptimeify_im_schedule.first_tier.id
  user_id     = data.uptimeify_im_schedule.first_tier.rotations[0].members[0]
  type        = "offline"
  starts_at   = "2030-08-01T00:00:00Z"
  ends_at     = "2030-08-15T00:00:00Z"
}
