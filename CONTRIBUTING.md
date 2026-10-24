# Contributing

Go, OpenTofu and Terraform run in Docker (`golang:1.25-alpine`); nothing needs to be installed on
the host.

```sh
make test      # go vet + unit tests
make docs      # regenerate docs/ with tfplugindocs from schema + examples/
make testacc   # acceptance tests against a real Uptimeify API, see below
```

## Acceptance tests

`scripts/acceptance.sh` runs `TF_ACC=1 go test ./internal/provider -run ^TestAcc` in a container
with a checksum-verified CLI: OpenTofu 1.11.14 by default, Terraform 1.17.0 with
`ACC_CLI=terraform`. It needs `UPTIMEIFY_API_TOKEN` and `UPTIMEIFY_ENDPOINT`. Tests create real
objects (named `tfacc-*`) and delete them again, so never point them at an organization you care
about. The script header describes how to get a token for a local test instance.

## Commits

No AI attribution in commit messages (`Co-Authored-By:`, `Claude-Session:`, "Generated with").
`.githooks/commit-msg` rejects them locally (`git config core.hooksPath .githooks`), CI rejects them
on every push.

## Release

Push a tag `v*`: `.github/workflows/release.yml` builds with GoReleaser, writes `SHA256SUMS` and
signs it with the GPG key from the repository secrets `GPG_PRIVATE_KEY` and `PASSPHRASE`.
