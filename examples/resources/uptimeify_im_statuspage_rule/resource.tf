# sev1 incidents of the on-call team set the ACME status page to degraded,
# and reset it when the incident resolves.
resource "uptimeify_im_statuspage_rule" "acme" {
  team_id        = tonumber(uptimeify_im_team.oncall.id)
  status_page_id = tonumber(uptimeify_status_page.acme.id)
  min_severity   = "sev1"
  target_state   = "degraded"
  auto_resolve   = true
}
