#!/usr/bin/env bash
# Acceptance tests (TF_ACC=1) against a real Uptimeify API, run with OpenTofu 1.11.14 inside a Go
# container. No Go or tofu on the host.
#
#   UPTIMEIFY_ENDPOINT=http://host.docker.internal:3001 UPTIMEIFY_API_TOKEN=wsm_... scripts/acceptance.sh
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
TOFU_VERSION=1.11.14
# From https://github.com/opentofu/opentofu/releases/download/v1.11.14/tofu_1.11.14_SHA256SUMS
TOFU_SHA256_amd64=8279a6c6d4044899ae77190fc50c40c147615a76aca13ea605d68320c424c2a6
TOFU_SHA256_arm64=3171726f7b1164af56fb67368742aa851162b8a9a60628ff73b8590e7f22dc73

# TF_ACC_PROVIDER_HOST/NAMESPACE: plugin-testing reattaches the in-process provider under
# <host>/<namespace>/uptimeify; the test config uses source "uptimeify/uptimeify", which OpenTofu
# resolves against registry.opentofu.org.
docker run --rm \
  ${ACC_DOCKER_NET:---add-host=host.docker.internal:host-gateway} \
  -v "$REPO:/src" -v tfp-gomod:/go/pkg/mod -v tfp-gocache:/root/.cache/go-build -v tfp-tofu:/opt/tofu \
  -w /src \
  -e UPTIMEIFY_ENDPOINT="$UPTIMEIFY_ENDPOINT" \
  -e UPTIMEIFY_API_TOKEN \
  $(env | grep -o '^UPTIMEIFY_ACC_[A-Z_]*' | sed 's/^/-e /') \
  -e TOFU_VERSION="$TOFU_VERSION" \
  -e TOFU_SHA256_amd64="$TOFU_SHA256_amd64" \
  -e TOFU_SHA256_arm64="$TOFU_SHA256_arm64" \
  -e TF_ACC=1 \
  -e TF_ACC_TERRAFORM_PATH=/opt/tofu/tofu \
  -e TF_ACC_PROVIDER_HOST=registry.opentofu.org \
  -e TF_ACC_PROVIDER_NAMESPACE=uptimeify \
  -e CHECKPOINT_DISABLE=1 \
  "$GO_IMAGE" sh -euc '
    arch=$(go env GOARCH)
    if [ "$(/opt/tofu/tofu version 2>/dev/null | head -1)" != "OpenTofu v$TOFU_VERSION" ]; then
      zip="tofu_${TOFU_VERSION}_linux_${arch}.zip"
      wget -qO "/tmp/$zip" "https://github.com/opentofu/opentofu/releases/download/v${TOFU_VERSION}/$zip"
      eval "want=\$TOFU_SHA256_$arch"
      echo "$want  /tmp/$zip" | sha256sum -c -
      unzip -oq "/tmp/$zip" tofu -d /opt/tofu
    fi
    /opt/tofu/tofu version | head -1
    go test -count=1 -v -timeout 30m ./internal/provider -run "^TestAcc"
  '
