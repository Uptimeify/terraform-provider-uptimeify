# Bob takes over for a release night, for the whole team.
resource "uptimeify_im_team_override" "release" {
  team_id   = uptimeify_im_team.platform.id
  user_id   = uptimeify_im_team_member.bob.user_id
  type      = "online"
  starts_at = "2030-09-01T18:00:00+02:00"
  ends_at   = "2030-09-02T06:00:00+02:00"
}
