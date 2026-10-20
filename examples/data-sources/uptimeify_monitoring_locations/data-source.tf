data "uptimeify_monitoring_locations" "all" {}

output "check_countries" {
  value = data.uptimeify_monitoring_locations.all.country_codes
}
