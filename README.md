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
