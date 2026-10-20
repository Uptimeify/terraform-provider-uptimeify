variable "ftp_password" {
  type      = string
  sensitive = true
}

resource "uptimeify_ftp_monitor" "this" {
  customer_id = 12
  name        = "Upload server"
  hostname    = "ftp.shop.example"
  user        = "monitor"
  tls_mode    = "explicit"

  password_wo         = var.ftp_password
  password_wo_version = 1
}
