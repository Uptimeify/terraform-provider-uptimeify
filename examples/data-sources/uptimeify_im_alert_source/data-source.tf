data "uptimeify_im_alert_source" "zabbix" {
  name = "Zabbix"
}

output "zabbix_title_selector" {
  value = data.uptimeify_im_alert_source.zabbix.payload_mapping.title
}
