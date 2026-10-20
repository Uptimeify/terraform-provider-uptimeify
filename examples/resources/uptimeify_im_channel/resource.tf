variable "slack_webhook_url" {
  type      = string
  sensitive = true
}

# Organization-wide Slack channel, also usable as the escalation fallback.
resource "uptimeify_im_channel" "ops_slack" {
  name               = "Ops Slack"
  type               = "slack"
  webhook_url_wo     = var.slack_webhook_url
  secrets_wo_version = 1 # bump to resend the URL after rotating it
}

# Team-scoped email channel.
resource "uptimeify_im_channel" "dba_mail" {
  name    = "DBA mailbox"
  type    = "email"
  team_id = 9
  email   = "dba-oncall@example.com"
}

resource "uptimeify_im_org_settings" "this" {
  fallback_channel_id = tonumber(uptimeify_im_channel.ops_slack.id)
}
