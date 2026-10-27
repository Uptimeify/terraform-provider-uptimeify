# A monitor of the page's customer
resource "uptimeify_status_page_component" "shop" {
  status_page_id = tonumber(uptimeify_status_page.acme.id)
  source_kind    = "monitor"
  monitor_type   = "http"
  monitor_id     = tonumber(uptimeify_website.shop.id)
  group_id       = uptimeify_status_page_component_group.core.group_id
}

# A status you set yourself
resource "uptimeify_status_page_component" "support" {
  status_page_id = tonumber(uptimeify_status_page.acme.id)
  source_kind    = "manual"
  name           = "Phone support"
  manual_status  = "operational"
}

# A third-party provider; follows its published status
resource "uptimeify_status_page_component" "github" {
  status_page_id  = tonumber(uptimeify_status_page.acme.id)
  source_kind     = "dependency"
  dependency_slug = "github"
}
