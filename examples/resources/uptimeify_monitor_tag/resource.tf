resource "uptimeify_monitor_tag" "shop_production" {
  monitor_type = "website"
  monitor_id   = 12
  tag_id       = tonumber(uptimeify_tag.production.id)
}
