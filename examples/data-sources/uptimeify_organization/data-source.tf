data "uptimeify_organization" "this" {}

output "organization_name" {
  value = data.uptimeify_organization.this.name
}
