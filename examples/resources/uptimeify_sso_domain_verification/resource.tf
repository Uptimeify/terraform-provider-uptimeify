# The TXT record from uptimeify_sso_oidc, published with your DNS provider.
resource "dns_txt_record_set" "sso" {
  zone = "example.com."
  name = trimsuffix(uptimeify_sso_oidc.corp.verification_record.name, ".example.com")
  txt  = [uptimeify_sso_oidc.corp.verification_record.value]
  ttl  = 300
}

# Verifies the TXT record once. Fails (no retry) while the record is not visible
# yet; apply again after DNS has propagated.
resource "uptimeify_sso_domain_verification" "corp" {
  domain     = uptimeify_sso_oidc.corp.domain
  depends_on = [dns_txt_record_set.sso]
}
