resource "uptimeify_status_page_component" "billing" {
  status_page_id = tonumber(uptimeify_status_page.acme.id)
  source_kind    = "manual"
  name           = "Billing backend"
}

# "Billing backend" is shown only to visitors signed in with a verified acme.com address
# (and to editors of the page); every other component stays visible to all visitors.
resource "uptimeify_status_page_audience_groups" "acme" {
  status_page_id = tonumber(uptimeify_status_page.acme.id)
  groups = [{
    name          = "ACME staff"
    email_domains = ["acme.com"]
    component_ids = [uptimeify_status_page_component.billing.component_id]
  }]
}
