# The TXT record from uptimeify_status_page_domain, published with your DNS provider.
resource "dns_txt_record_set" "acme_status" {
  zone = "acme.example."
  name = trimsuffix(uptimeify_status_page_domain.acme.verification_record.name, ".acme.example")
  txt  = [uptimeify_status_page_domain.acme.verification_record.value]
  ttl  = 300
}

# Verifies the TXT record and activates the domain once. Fails (no retry) while the
# record is not visible yet; apply again after DNS has propagated.
resource "uptimeify_status_page_domain_activation" "acme" {
  domain_id  = tonumber(uptimeify_status_page_domain.acme.id)
  depends_on = [dns_txt_record_set.acme_status]
}
