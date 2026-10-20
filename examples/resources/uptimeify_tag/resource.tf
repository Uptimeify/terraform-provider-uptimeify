resource "uptimeify_tag" "production" {
  name  = "production"
  color = "red"
}

# Scoped to one customer instead of the whole organization.
resource "uptimeify_tag" "customer" {
  name        = "eu-shop"
  customer_id = 12
}
