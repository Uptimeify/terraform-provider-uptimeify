#!/usr/bin/env bash
# Acceptance tests (TF_ACC=1) against a real Uptimeify API, run with OpenTofu 1.11.14 (default) or
# Terraform 1.17.0 (ACC_CLI=terraform) inside a Go container. No Go, tofu or terraform on the host.
#
#   UPTIMEIFY_ENDPOINT=http://host.docker.internal:3001 UPTIMEIFY_API_TOKEN=wsm_... scripts/acceptance.sh
#   ACC_CLI=terraform UPTIMEIFY_API_TOKEN=wsm_... scripts/acceptance.sh
#
# Against the website-monitoring sandbox: `make sandbox` + `make sandbox-app` there (app on host
# :3001), so from this container the endpoint is http://host.docker.internal:3001 (the default).
#
# Org token wsm_... for the sandbox, shortest way: insert one directly into the sandbox DB. The API
# stores only sha256(token) as hex (server/lib/auth/api-tokens.ts hashApiToken; sandbox/mcp-acceptance.ts
# mints its tokens the same way). created_by = the seeded admin of org 1, so the token runs as
# 'admin' (server/lib/auth/api-token-role.ts):
#
#   T="wsm_$(openssl rand -hex 32)"
#   docker exec -i uptimeify-sandbox-postgres psql -U sandbox -d sandbox -qAt -c \
#     "insert into api_token (organization_id, name, token, created_by)
#      values (1, 'terraform-acc', encode(sha256(convert_to('$T', 'UTF8')), 'hex'), 'sandbox-user-1')"
#   export UPTIMEIFY_API_TOKEN="$T"
#   # clean up: delete from api_token where name = 'terraform-acc'; delete from tag where name like 'tfacc-%';
#
# Via the API instead (same result, more steps): sign in as sandbox@example.test / sandbox-password
# (better-auth POST /api/auth/sign-in/email, keep the session cookie), then
# POST /api/organization/tokens {"name":"terraform-acc"} with that cookie; the plaintext token is
# only in that response. Needs an org admin and passes the MFA step-up (the seed user has no MFA).
set -euo pipefail

: "${UPTIMEIFY_API_TOKEN:?set UPTIMEIFY_API_TOKEN (see header of this script)}"
UPTIMEIFY_ENDPOINT="${UPTIMEIFY_ENDPOINT:-http://host.docker.internal:3001}"
# ACC_DOCKER_NET: Docker-Netzoption des Testcontainers. Die Nuxt-Dev-App lehnt den Host
# host.docker.internal ab (Vite allowedHosts); website-monitoring setzt daher
# "--network container:<app>" und UPTIMEIFY_ENDPOINT=http://localhost:3000.

REPO="$(cd "$(dirname "$0")/.." && pwd)"
GO_IMAGE="${GO_IMAGE:-golang:1.25-alpine}"
ACC_CLI="${ACC_CLI:-tofu}"
case "$ACC_CLI" in
  tofu)
    CLI_VERSION=1.11.14
    CLI_BASE="https://github.com/opentofu/opentofu/releases/download/v${CLI_VERSION}/tofu_${CLI_VERSION}"
    # From https://github.com/opentofu/opentofu/releases/download/v1.11.14/tofu_1.11.14_SHA256SUMS
    CLI_SHA256_amd64=8279a6c6d4044899ae77190fc50c40c147615a76aca13ea605d68320c424c2a6
    CLI_SHA256_arm64=3171726f7b1164af56fb67368742aa851162b8a9a60628ff73b8590e7f22dc73
    CLI_BANNER="OpenTofu v$CLI_VERSION"
    PROVIDER_HOST=registry.opentofu.org ;;
  terraform)
    CLI_VERSION=1.17.0
    CLI_BASE="https://releases.hashicorp.com/terraform/${CLI_VERSION}/terraform_${CLI_VERSION}"
    # From https://releases.hashicorp.com/terraform/1.17.0/terraform_1.17.0_SHA256SUMS
    CLI_SHA256_amd64=c168cab7c637a8769fdb90089ddd19de701bc6ff8aca64dc2e2185fe46f59f67
    CLI_SHA256_arm64=f32478134ffc6d710eec474bbd316652cdddc0f4e336973a3745e8d3a3309cdf
    CLI_BANNER="Terraform v$CLI_VERSION"
    PROVIDER_HOST=registry.terraform.io ;;
  *) echo "ACC_CLI must be tofu or terraform" >&2; exit 2 ;;
esac

# TF_ACC_PROVIDER_HOST/NAMESPACE: plugin-testing reattaches the in-process provider under
# <host>/<namespace>/uptimeify; the test config uses source "uptimeify/uptimeify", which each CLI
# resolves against its own default registry.
docker run --rm \
  ${ACC_DOCKER_NET:---add-host=host.docker.internal:host-gateway} \
  -v "$REPO:/src" -v tfp-gomod:/go/pkg/mod -v tfp-gocache:/root/.cache/go-build -v "tfp-$ACC_CLI:/opt/cli" \
  -w /src \
  -e UPTIMEIFY_ENDPOINT="$UPTIMEIFY_ENDPOINT" \
  -e UPTIMEIFY_API_TOKEN \
  $(env | grep -o '^UPTIMEIFY_ACC_[A-Z_]*' | sed 's/^/-e /') \
  -e ACC_CLI="$ACC_CLI" \
  -e CLI_BASE="$CLI_BASE" \
  -e CLI_BANNER="$CLI_BANNER" \
  -e CLI_SHA256_amd64="$CLI_SHA256_amd64" \
  -e CLI_SHA256_arm64="$CLI_SHA256_arm64" \
  -e TF_ACC=1 \
  -e TF_ACC_TERRAFORM_PATH="/opt/cli/$ACC_CLI" \
  -e TF_ACC_PROVIDER_HOST="$PROVIDER_HOST" \
  -e TF_ACC_PROVIDER_NAMESPACE=uptimeify \
  -e CHECKPOINT_DISABLE=1 \
  "$GO_IMAGE" sh -euc '
    arch=$(go env GOARCH)
    if [ "$(/opt/cli/$ACC_CLI version 2>/dev/null | head -1)" != "$CLI_BANNER" ]; then
      zip="${CLI_BASE##*/}_linux_${arch}.zip"
      wget -qO "/tmp/$zip" "${CLI_BASE}_linux_${arch}.zip"
      eval "want=\$CLI_SHA256_$arch"
      echo "$want  /tmp/$zip" | sha256sum -c -
      unzip -oq "/tmp/$zip" "$ACC_CLI" -d /opt/cli
    fi
    /opt/cli/$ACC_CLI version | head -1
    go test -count=1 -v -timeout 30m ./internal/provider -run "^TestAcc"
  '
