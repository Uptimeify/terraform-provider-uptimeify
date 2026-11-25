resource "uptimeify_customer_ip" "mail" {
  customer_id = tonumber(uptimeify_customer.shop.id)
  ip_address  = "8.8.4.4"
  label       = "Mail server"
}
