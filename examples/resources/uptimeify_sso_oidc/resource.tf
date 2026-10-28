variable "oidc_client_secret" {
  type      = string
  sensitive = true
}

# Register the client at your identity provider first, with
# uptimeify_sso_oidc.corp.redirect_uri and the scopes openid, email, profile.
resource "uptimeify_sso_oidc" "corp" {
  issuer                   = "https://login.example.com/realms/corp"
  client_id                = "uptimeify"
  client_secret_wo         = var.oidc_client_secret
  client_secret_wo_version = 1
  domain                   = "example.com"
  default_role             = "editor"
  # enforce_sso = true  # only after the domain is verified and one SSO login worked
}
