// Command tf-technitium exports a Technitium DNS Server to Terraform
// configuration: resource blocks for what is on the server, and import
// blocks that adopt it on the next apply.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/ilijamt/terraform-provider-technitium/internal/terraforming"
	"github.com/ilijamt/terraform-provider-technitium/version"
)

func main() {
	insecure, _ := strconv.ParseBool(os.Getenv("TECHNITIUM_INSECURE"))
	o := terraforming.Options{
		Host:     os.Getenv("TECHNITIUM_HOST"),
		Token:    os.Getenv("TECHNITIUM_TOKEN"),
		Username: os.Getenv("TECHNITIUM_USERNAME"),
		Password: os.Getenv("TECHNITIUM_PASSWORD"),
		Insecure: insecure,
		Version:  version.Version,
		Log:      os.Stderr,
	}
	var types, zones string
	flag.StringVar(&o.Host, "host", o.Host, "API address, e.g. http://localhost:5380 (TECHNITIUM_HOST)")
	flag.StringVar(&o.Username, "username", o.Username, "user to log in as without a token (TECHNITIUM_USERNAME); the password comes from TECHNITIUM_PASSWORD")
	flag.BoolVar(&o.Insecure, "insecure", o.Insecure, "skip TLS certificate verification (TECHNITIUM_INSECURE)")
	flag.StringVar(&o.Out, "out", ".", "directory to write the files to")
	flag.BoolVar(&o.Force, "force", false, "overwrite existing files")
	flag.StringVar(&types, "type", "", "comma separated resource types to export, without technitium_; a trailing * matches a prefix (record_*)")
	flag.StringVar(&zones, "zone", "", "comma separated zones to export with their records")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `Usage: tf-technitium [flags]

Writes Terraform configuration for everything on a Technitium DNS Server,
one file per group of resource types, imports.tf with import blocks that
adopt the objects on the next apply, and variables.tf for secrets.

The token or password come from TECHNITIUM_TOKEN or TECHNITIUM_PASSWORD.

`)
		flag.PrintDefaults()
	}
	flag.Parse()
	o.Types, o.Zones = split(types), split(zones)

	switch {
	case o.Host == "":
		fail("set -host or TECHNITIUM_HOST")
	case o.Token == "" && (o.Username == "" || o.Password == ""):
		fail("set TECHNITIUM_TOKEN, or -username and TECHNITIUM_PASSWORD")
	case o.Token != "":
		o.Username, o.Password = "", ""
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := terraforming.Run(ctx, o); err != nil {
		fail(err.Error())
	}
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimPrefix(strings.TrimSpace(p), "technitium_"); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "tf-technitium:", msg)
	os.Exit(1)
}
