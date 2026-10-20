# One per organization; destroy only removes it from the state.
resource "uptimeify_im_org_settings" "this" {
  monitoring_bridge   = "both"
  overage_enabled     = false
  fallback_channel_id = 3
}
