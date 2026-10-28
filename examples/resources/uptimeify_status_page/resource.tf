data "uptimeify_customer" "acme" {
  name = "ACME GmbH"
}

# Shows the explicit components and every other monitor of the customer except the hidden ones.
resource "uptimeify_status_page" "acme" {
  customer_id               = tonumber(data.uptimeify_customer.acme.id)
  name                      = "ACME Status"
  slug                      = "acme"
  description               = "Live status of the ACME shop."
  include_customer_monitors = true

  hidden_monitors = [
    { type = "http", id = 1234 },
    { type = "dns", id = 56 },
  ]
}

# A page of the whole organization: no customer, only the explicit components
# (uptimeify_status_page_component), which may come from any customer.
resource "uptimeify_status_page" "platform" {
  name = "Platform Status"
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
