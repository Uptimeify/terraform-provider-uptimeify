resource "uptimeify_customer_ip" "mail" {
  customer_id = tonumber(uptimeify_customer.shop.id)
  ip_address  = "203.0.113.25"
  label       = "Mail server"
}
