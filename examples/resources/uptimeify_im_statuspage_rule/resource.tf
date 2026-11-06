# sev1 incidents of the on-call team set the ACME status page to degraded,
# and reset it when the incident resolves. message is the public text until the
# incident gets its first status page update.
resource "uptimeify_im_statuspage_rule" "acme" {
  team_id        = tonumber(uptimeify_im_team.oncall.id)
  status_page_id = tonumber(uptimeify_status_page.acme.id)
  min_severity   = "sev1"
  target_state   = "degraded"
  auto_resolve   = true
  message        = "We are investigating an outage and will post updates here."
}
