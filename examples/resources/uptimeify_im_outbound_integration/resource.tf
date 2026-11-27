variable "teams_webhook_url" {
  type      = string
  sensitive = true
}

variable "jira_api_token" {
  type      = string
  sensitive = true
}

# Post every sev1/sev2 incident event of the platform team to a Microsoft Teams channel.
resource "uptimeify_im_outbound_integration" "teams" {
  name               = "Platform Teams"
  type               = "teams"
  team_id            = 7
  filter_severities  = ["sev1", "sev2"]
  webhook_url_wo     = var.teams_webhook_url
  secrets_wo_version = 1
}

# Open a Jira issue per new incident (the server default event_kinds for jira).
resource "uptimeify_im_outbound_integration" "jira" {
  name         = "Jira OPS"
  type         = "jira"
  email        = "bot@example.com"
  base_url     = "https://example.atlassian.net"
  project_key  = "OPS"
  api_token_wo = var.jira_api_token
}

variable "linear_api_key" {
  type      = string
  sensitive = true
}

variable "webhook_signing_secret" {
  type      = string
  sensitive = true
}

# Post sev1 incidents to a Slack channel through the Uptimeify Slack app. The workspace is connected
# once in the Uptimeify UI (Slack OAuth); its id is shown there. sev1 incidents also get their own
# private channel.
resource "uptimeify_im_outbound_integration" "slack_app" {
  name                          = "Slack #incidents"
  type                          = "slack_app"
  workspace_id                  = 3
  channel_id                    = "C0123ABCD"
  filter_severities             = ["sev1"]
  incident_channel_min_severity = "sev1"
  incident_channel_private      = true
}

# Open a Linear issue per new incident.
resource "uptimeify_im_outbound_integration" "linear" {
  name           = "Linear ENG"
  type           = "linear"
  linear_team_id = "9cfb482a-81e3-4154-b5b9-2c805e70a02d"
  event_kinds    = ["incident_created"]
  api_key_wo     = var.linear_api_key
}

# Signed generic webhook: every delivery carries X-Webhook-Signature (HMAC-SHA256 of the body).
resource "uptimeify_im_outbound_integration" "signed_hook" {
  name               = "Signed webhook"
  type               = "webhook"
  webhook_url_wo     = "https://hooks.example.com/uptimeify"
  signing_secret_wo  = var.webhook_signing_secret
  secrets_wo_version = 1
}
