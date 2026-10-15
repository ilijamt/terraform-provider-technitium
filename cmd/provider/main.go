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
	var address string
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.StringVar(&address, "address", "registry.terraform.io/ilijamt/technitium", "provider address to serve under, e.g. registry.opentofu.org/ilijamt/technitium to attach a debugger from tofu")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version.Version, nil, technitium.Resources(), technitium.DataSources()), providerserver.ServeOpts{
		Address: address,
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
