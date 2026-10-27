resource "uptimeify_status_page_component_group" "core" {
  status_page_id = tonumber(uptimeify_status_page.acme.id)
  name           = "Core services"
  sort           = 0
}
