resource "uptimeify_im_service" "webshop" {
  name          = "Webshop"
  description   = "Checkout and product pages."
  criticality   = "critical"
  owner_team_id = tonumber(uptimeify_im_team.platform.id)

  monitors = [
    { monitor_type = "website", monitor_id = tonumber(uptimeify_website.shop.id) },
    { monitor_type = "tcp", monitor_id = tonumber(uptimeify_tcp_monitor.db.id) },
  ]
}
