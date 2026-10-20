package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/provider"
)

// Generate docs/ from the schema and examples/ (tfplugindocs layout).
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 generate --provider-name uptimeify

// version is set by goreleaser via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/uptimeify/uptimeify",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
