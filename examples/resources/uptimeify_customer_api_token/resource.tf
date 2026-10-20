resource "uptimeify_customer_api_token" "shop" {
  name            = "Shop integration"
  customer_id     = 12
  expires_in_days = 365
}

output "shop_token" {
  value     = uptimeify_customer_api_token.shop.token
  sensitive = true
}
