resource "uptimeify_customer_domain" "shop" {
  customer_id         = tonumber(uptimeify_customer.shop.id)
  domain_name         = "example-shop.com"
  expiry_warning_days = 45
}
