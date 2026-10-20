# Note: the API only lets a signed-in admin manage organization tokens; with a wsm_ provider
# token this resource fails with an explanatory error.
resource "uptimeify_api_token" "ci" {
  name            = "CI pipeline"
  expires_in_days = 90
}
