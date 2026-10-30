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
  management_type           = "self_service" # the customer operates it; default "managed"

  hidden_monitors = [
    { type = "http", id = 1234 },
    { type = "dns", id = 56 },
  ]
}

# A page of the whole organization: no customer, only the explicit components
# (uptimeify_status_page_component), which may come from any customer. Visitors may also
# subscribe by SMS and browser push.
resource "uptimeify_status_page" "platform" {
  name                       = "Platform Status"
  sms_subscriptions_enabled  = true
  push_subscriptions_enabled = true
}

# Internal page: only members signed in through the organization's single sign-on, and only
# from the office networks.
resource "uptimeify_status_page" "staff" {
  name         = "Staff Status"
  visibility   = "sso_members_only"
  ip_allowlist = ["203.0.113.0/24", "2001:db8::/32"]
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
