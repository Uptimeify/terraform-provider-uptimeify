# Who holds the pager of team 7 right now, per escalation tier.
data "uptimeify_im_on_call" "platform" {
  team_id = 7
}

output "first_tier_on_call" {
  value = [for u in data.uptimeify_im_on_call.platform.tiers[0].users : u.user_name]
}
