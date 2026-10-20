data "uptimeify_user" "alice" {
  email = "alice@example.com"
}

resource "uptimeify_im_team_member" "alice" {
  team_id = uptimeify_im_team.platform.id
  user_id = data.uptimeify_user.alice.id
  im_role = "admin"
}

# Invite by email: an unknown address gets an account and an invitation mail.
resource "uptimeify_im_team_member" "bob" {
  team_id      = uptimeify_im_team.platform.id
  invite_email = "bob@example.com"
}
