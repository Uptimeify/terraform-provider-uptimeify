data "uptimeify_customer" "shop" {
  name = "Example Shop GmbH"
}

resource "uptimeify_dns_monitor" "this" {
  customer_id     = data.uptimeify_customer.shop.id
  name            = "shop.example MX"
  hostname        = "shop.example"
  record_types    = ["A", "MX"]
  match_mode      = "contains"
  expected_values = {
    A  = ["203.0.113.10"]
    MX = ["10 mail.shop.example."]
  }
  allowed_check_country_codes = ["DE", "NL"]
}
