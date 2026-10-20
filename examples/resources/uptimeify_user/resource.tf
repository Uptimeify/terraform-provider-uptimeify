# Sends an invitation mail; the person sets their own password.
resource "uptimeify_user" "ops" {
  email      = "ops@example.com"
  first_name = "Olivia"
  last_name  = "Ops"
  role       = "editor"
}

# A customer login, limited to one customer.
resource "uptimeify_user" "shop_owner" {
  email        = "owner@shop.example"
  role         = "readonly"
  customer_ids = [12]
  language     = "en"
}
