variable "mailbox_password" {
  type      = string
  sensitive = true
}

resource "uptimeify_imap_pop_monitor" "this" {
  customer_id = 12
  name        = "Support mailbox"
  hostname    = "imap.shop.example"
  protocol    = "imap"
  tls         = true
  user        = "support@shop.example"

  password_wo         = var.mailbox_password
  password_wo_version = 1
}
