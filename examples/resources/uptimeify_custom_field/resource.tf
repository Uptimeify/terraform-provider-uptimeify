resource "uptimeify_custom_field" "contract" {
  name      = "Contract"
  field_key = "contract"
}

resource "uptimeify_custom_field" "tier" {
  name       = "Support tier"
  field_type = "select"
  options    = ["basic", "premium"]
}
