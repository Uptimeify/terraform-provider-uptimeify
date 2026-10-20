resource "uptimeify_status_page_domain" "acme" {
  status_page_id = tonumber(uptimeify_status_page.acme.id)
  hostname       = "status.acme.example"
}

# Publish this TXT record in your DNS zone (any DNS provider), e.g.:
output "acme_status_txt" {
  value = uptimeify_status_page_domain.acme.verification_record
}
