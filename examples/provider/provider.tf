terraform {
  required_version = ">= 1.11"
  required_providers {
    uptimeify = {
      source = "uptimeify/uptimeify"
    }
  }
}

# endpoint and api_token fall back to UPTIMEIFY_ENDPOINT and UPTIMEIFY_API_TOKEN.
provider "uptimeify" {}
