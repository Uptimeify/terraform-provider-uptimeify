resource "uptimeify_icmp_monitor" "this" {
  customer_id = 12
  name        = "Edge router"
  hostname    = "gw.shop.example"
  ping_count  = 3
}
