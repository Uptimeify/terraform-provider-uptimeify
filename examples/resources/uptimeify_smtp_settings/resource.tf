variable "smtp_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "uptimeify_smtp_settings" "this" {
  host                = "smtp.example.com"
  port                = 587
  username            = "alerts@example.com"
  password_wo         = var.smtp_password
  password_wo_version = 1 # bump to send a new password
  tls_mode            = "starttls"
  from_name           = "Example Monitoring"
  from_email          = "alerts@example.com"
}
