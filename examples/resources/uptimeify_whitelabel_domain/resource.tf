resource "uptimeify_whitelabel_domain" "app" {
  hostname = "monitoring.example.com"
}

# Publish this TXT record, then verify once:
# POST /api/organization/whitelabel/domains/verify { "domainId": <id> }
output "txt_record" {
  value = "${uptimeify_whitelabel_domain.app.dns_txt_name} TXT ${uptimeify_whitelabel_domain.app.dns_txt_value}"
}
