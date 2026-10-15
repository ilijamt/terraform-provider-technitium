package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/ilijamt/terraform-provider-technitium/internal/provider"
	"github.com/ilijamt/terraform-provider-technitium/internal/technitium"
	"github.com/ilijamt/terraform-provider-technitium/version"
)

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version.Version, nil, technitium.Resources(), technitium.DataSources()), providerserver.ServeOpts{
		Address: "registry.terraform.io/ilijamt/technitium",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
