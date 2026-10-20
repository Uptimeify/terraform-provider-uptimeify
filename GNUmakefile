# Everything runs in Docker; no Go on the host.
GO_IMAGE ?= golang:1.25-alpine
DOCKER_GO = docker run --rm -v $(CURDIR):/src -v tfp-gomod:/go/pkg/mod -v tfp-gocache:/root/.cache/go-build -w /src $(GO_IMAGE)

.PHONY: build test testacc docs

build:
	$(DOCKER_GO) go build ./...

test:
	$(DOCKER_GO) sh -c 'go vet ./... && go test ./...'

# Needs UPTIMEIFY_ENDPOINT and UPTIMEIFY_API_TOKEN, see scripts/acceptance.sh.
testacc:
	scripts/acceptance.sh

docs:
	$(DOCKER_GO) go generate ./...
