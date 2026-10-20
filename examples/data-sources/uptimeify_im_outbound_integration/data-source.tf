data "uptimeify_im_outbound_integration" "jira" {
  name = "Jira OPS"
}

output "jira_project" {
  value = data.uptimeify_im_outbound_integration.jira.project_key
}
