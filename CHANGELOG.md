# Changelog

## Unreleased

- `uptimeify_im_outbound_integration` (resource and data source): new types `slack_app`,
  `mattermost`, `googlechat` and `linear`; new attributes `signing_secret_wo` and `action_links`
  (`webhook`), `minimal_payload` (chat webhooks), `workspace_id`, `channel_id`, `read_only`,
  `thread_comment_sync`, `incident_channel_min_severity`, `incident_channel_private` (`slack_app`),
  `linear_team_id` (`linear`, with `api_key_wo`), and the read-only flags `has_signing_secret` and
  `has_inbound_token`. A `slack_app` integration needs a Slack workspace connected in the Uptimeify
  UI; Terraform references it by `workspace_id`.
