# Alice is on vacation, Bob covers.
resource "uptimeify_im_schedule_override" "vacation" {
  schedule_id         = uptimeify_im_schedule.weekly.id
  user_id             = uptimeify_im_team_member.alice.user_id
  type                = "offline"
  covered_by_user_ids = [uptimeify_im_team_member.bob.user_id]
  starts_at           = "2030-08-01T00:00:00Z"
  ends_at             = "2030-08-15T00:00:00Z"
}
