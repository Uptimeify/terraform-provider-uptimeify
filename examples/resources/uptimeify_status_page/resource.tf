data "uptimeify_customer" "acme" {
  name = "ACME GmbH"
}

# Shows every monitor of the customer except the hidden ones.
resource "uptimeify_status_page" "acme" {
  customer_id = tonumber(data.uptimeify_customer.acme.id)
  name        = "ACME Status"
  slug        = "acme"
  description = "Live status of the ACME shop."

  hidden_monitors = [
    { type = "http", id = 1234 },
    { type = "dns", id = 56 },
  ]
}

# Password-protected page; the password never lands in state.
# Bump access_password_wo_version to send a new password.
resource "uptimeify_status_page" "internal" {
  customer_id                = tonumber(data.uptimeify_customer.acme.id)
  name                       = "ACME Internal"
  visibility                 = "password_protected"
  access_password_wo         = var.status_page_password
  access_password_wo_version = 1
}

variable "status_page_password" {
  type      = string
  sensitive = true
  ephemeral = true
}
