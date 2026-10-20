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
