data "uptimeify_package_configs" "all" {}

output "package_types" {
  value = [for p in data.uptimeify_package_configs.all.package_configs : p.package_type]
}
