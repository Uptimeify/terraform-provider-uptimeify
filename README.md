# Uptimeify Provider for OpenTofu and Terraform

Your monitoring, defined in code. Monitors, incident management, status pages and users live next
to the infrastructure they watch, go through the same pull request review, and roll out the same
way for every customer. Drift shows up in the next `plan`, not in the next outage.

Works with OpenTofu 1.11+ and Terraform 1.11+. Every resource is acceptance-tested against a real
Uptimeify instance with OpenTofu 1.11.14 and Terraform 1.17.0.

## Quick start

```hcl
terraform {
  required_version = ">= 1.11"
  required_providers {
    uptimeify = {
      source  = "uptimeify/uptimeify"
      version = "~> 0.1"
    }
  }
}

provider "uptimeify" {} # reads UPTIMEIFY_API_TOKEN

data "uptimeify_customer" "shop" {
  name = "Example Shop GmbH"
}

variable "api_key" {
  type      = string
  sensitive = true
}

resource "uptimeify_website" "api" {
  customer_id           = tonumber(data.uptimeify_customer.shop.id)
  name                  = "Shop API health"
  url                   = "https://shop.example.com/api/health"
  check_interval        = 5
  check_keyword_enabled = true
  search_term           = "\"status\":\"ok\""

  # Write-only: never stored in state, never returned by the API.
  custom_headers_wo         = [{ name = "X-Api-Key", value = var.api_key }]
  custom_headers_wo_version = 1
}
```

```sh
export UPTIMEIFY_API_TOKEN=wsm_...
tofu init && tofu apply      # or: terraform init && terraform apply
```

## Authentication

| Setting     | Environment variable  | Default                |
| ----------- | --------------------- | ---------------------- |
| `api_token` | `UPTIMEIFY_API_TOKEN` | none (required)        |
| `endpoint`  | `UPTIMEIFY_ENDPOINT`  | `https://uptimeify.io` |

API tokens start with `wsm_` and come in two scopes. An organization token manages the whole
organization. A customer token reaches only the customers it is scoped to: the right fit when you
run monitoring for several clients.

**One configuration per client.** Give each client its own provider alias and customer token. A
mistake in one client's code cannot touch another client's monitors.

```hcl
provider "uptimeify" {
  alias     = "acme"
  api_token = var.acme_token
}

resource "uptimeify_website" "acme_shop" {
  provider = uptimeify.acme
  # ...
}
```

With Terragrunt, generate the `provider` block and pass the token per unit, as with any other
provider.

## Bring existing monitors under code

Every resource supports `import`. Monitors take their numeric id or public id; the ID format of
every other resource is in its docs.

```sh
tofu import uptimeify_website.api 42
```

Write-only secrets (headers, request bodies, passwords, HMAC keys) are not imported. Set them in the
configuration together with their `*_wo_version` after the import.

## What you can manage

Everything the Uptimeify API allows an API token to change: 44 resources and 25 data sources.

| Area | Resources |
| --- | --- |
| Monitors | `website` (HTTP and heartbeat), `dns_monitor`, `tcp_monitor`, `icmp_monitor`, `smtp_monitor`, `ssh_monitor`, `ftp_monitor`, `imap_pop_monitor` |
| Organizing | `tag`, `monitor_tag`, `custom_field`, `maintenance_window` |
| Alerting | `notification_channel`, `escalation_config` |
| Customers | `customer`, `customer_ip`, `customer_domain` |
| Incident management | `im_team`, `im_team_member`, `im_escalation_tier`, `im_schedule`, `im_schedule_override`, `im_team_override`, `im_alert_source`, `im_routing_rule`, `im_channel`, `im_outbound_integration`, `im_statuspage_rule`, `im_org_settings` |
| Status pages | `status_page`, `status_page_design`, `status_page_component`, `status_page_component_group`, `status_page_maintenance`, `status_page_domain`, `status_page_domain_activation` |
| Organization | `organization_settings`, `organization_branding`, `whitelabel_domain`, `report`, `smtp_settings`, `user`, `sso_oidc`, `sso_domain_verification` |

All names carry the `uptimeify_` prefix. Data sources cover the monitors and the lookups you need to
wire things together, for example `uptimeify_customer` and `uptimeify_im_team` by name,
`uptimeify_im_on_call`, `uptimeify_monitoring_locations` and `uptimeify_package_configs`.
Full reference for every attribute: [`docs/`](docs/).

**Secrets stay out of your state.** Every secret is a write-only attribute (`*_wo`). Increase the
matching `*_wo_version` to send a new value.

**`apply` and `destroy` act for real.** `uptimeify_user` sends the invitation email on create and
deletes the user on destroy.

Not available through any API token, and therefore not in the provider: API tokens themselves,
personal notification settings, the organization's MFA policy and the SMS overage limit.

## Support

API documentation: [docs.uptimeify.io/api](https://docs.uptimeify.io/api).
Questions and feature requests: [hello@uptimeify.io](mailto:hello@uptimeify.io).
Development and releases: [CONTRIBUTING.md](CONTRIBUTING.md).

License: MPL-2.0.
