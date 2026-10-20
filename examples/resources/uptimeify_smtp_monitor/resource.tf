variable "smtp_password" {
  type      = string
  sensitive = true
}

resource "uptimeify_smtp_monitor" "this" {
  customer_id = 12
  name        = "Outgoing mail"
  hostname    = "mail.shop.example"
  port        = 587
  auth_user   = "monitor@shop.example"

  # Write-only: never stored in state. Bump the version to send a new password.
  auth_pass_wo         = var.smtp_password
  auth_pass_wo_version = 1
}
