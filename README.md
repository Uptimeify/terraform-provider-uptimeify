# Terraform / OpenTofu Provider for Uptimeify

Manage [Uptimeify](https://uptimeify.io) resources as code. Plugin protocol 6, requires
OpenTofu >= 1.11 or Terraform >= 1.11.

```hcl
terraform {
  required_providers {
    uptimeify = { source = "uptimeify/uptimeify" }
  }
}

provider "uptimeify" {} # reads UPTIMEIFY_ENDPOINT / UPTIMEIFY_API_TOKEN

resource "uptimeify_tag" "production" {
  name  = "production"
  color = "red"
}
```

| Setting     | Env                    | Default                |
| ----------- | ---------------------- | ---------------------- |
| `endpoint`  | `UPTIMEIFY_ENDPOINT`   | `https://uptimeify.io` |
| `api_token` | `UPTIMEIFY_API_TOKEN`  | none (required)        |

`api_token` is an organization token (`wsm_...`) or a customer token, sent as
`Authorization: Bearer <token>`. Resources and data sources: see [`docs/`](docs/).

## Coverage

Everything the Uptimeify API offers to an API token is manageable here (OpenTofu 1.11+ / Terraform 1.11+). Secrets are write-only attributes (`*_wo` with a `*_wo_version` trigger) and never land in the state.

**Resources (38):** `uptimeify_custom_field`, `uptimeify_customer`, `uptimeify_customer_domain`, `uptimeify_customer_ip`, `uptimeify_dns_monitor`, `uptimeify_escalation_config`, `uptimeify_ftp_monitor`, `uptimeify_icmp_monitor`, `uptimeify_im_alert_source`, `uptimeify_im_channel`, `uptimeify_im_escalation_tier`, `uptimeify_im_org_settings`, `uptimeify_im_outbound_integration`, `uptimeify_im_routing_rule`, `uptimeify_im_schedule`, `uptimeify_im_schedule_override`, `uptimeify_im_statuspage_rule`, `uptimeify_im_team`, `uptimeify_im_team_member`, `uptimeify_im_team_override`, `uptimeify_imap_pop_monitor`, `uptimeify_maintenance_window`, `uptimeify_monitor_tag`, `uptimeify_notification_channel`, `uptimeify_organization_settings`, `uptimeify_report`, `uptimeify_smtp_monitor`, `uptimeify_smtp_settings`, `uptimeify_ssh_monitor`, `uptimeify_status_page`, `uptimeify_status_page_design`, `uptimeify_status_page_domain`, `uptimeify_status_page_domain_activation`, `uptimeify_tag`, `uptimeify_tcp_monitor`, `uptimeify_user`, `uptimeify_website`, `uptimeify_whitelabel_domain`

**Data sources (25):** `uptimeify_custom_field`, `uptimeify_customer`, `uptimeify_dns_monitor`, `uptimeify_ftp_monitor`, `uptimeify_icmp_monitor`, `uptimeify_im_alert_source`, `uptimeify_im_channel`, `uptimeify_im_on_call`, `uptimeify_im_outbound_integration`, `uptimeify_im_routing_rule`, `uptimeify_im_statuspage_rule`, `uptimeify_im_team`, `uptimeify_imap_pop_monitor`, `uptimeify_maintenance_window`, `uptimeify_monitoring_locations`, `uptimeify_notification_channel`, `uptimeify_organization`, `uptimeify_package_configs`, `uptimeify_smtp_monitor`, `uptimeify_ssh_monitor`, `uptimeify_status_page`, `uptimeify_tag`, `uptimeify_tcp_monitor`, `uptimeify_user`, `uptimeify_website`

Not available, because the API refuses them for API tokens: API tokens themselves, personal incident-management notification settings, the organization's MFA requirements and the SMS overage cap (global admins only).

## Development

No Go on the host; everything runs in Docker (`golang:1.25-alpine`):

```sh
make test      # go vet + unit tests
make docs      # regenerate docs/ with tfplugindocs from schema + examples/
make testacc   # acceptance tests with OpenTofu 1.11.14, see below
```

## Acceptance tests

`scripts/acceptance.sh` runs `TF_ACC=1 go test ./internal/provider -run ^TestAcc` in a container
with a checksum-verified OpenTofu 1.11.14. It needs `UPTIMEIFY_API_TOKEN` and, optionally,
`UPTIMEIFY_ENDPOINT` (default `http://host.docker.internal:3001`, the website-monitoring sandbox
app). The script header shows how to mint a sandbox org token. Tests create real objects
(tags named `tfacc-*`) and delete them again.

## Release

Push a tag `v*`: `.github/workflows/release.yml` builds with goreleaser, writes `SHA256SUMS` and
signs it with the GPG key from the secrets `GPG_PRIVATE_KEY` / `PASSPHRASE`.

License: MPL-2.0.
