data "uptimeify_im_channel" "ops" {
  name = "Ops Slack"
}

output "ops_channel_id" {
  value = data.uptimeify_im_channel.ops.id
}
