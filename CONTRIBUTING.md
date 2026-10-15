Development
===========

```shell
make build      # ./build/terraform-provider-technitium and ./build/tf-technitium
make generate   # render docs/ from the schemas, examples/ and templates/
make test       # unit tests
make testacc    # acceptance tests against a technitium/dns-server container
make lint       # gofmt, terraform fmt, go vet and golangci-lint
```

`make testacc` needs Docker. It starts the image named by
`technitium.Version`, or uses the server at `TECHNITIUM_HOST` when that's set,
which must accept `admin`/`admin`. If a run is killed, `make testacc-clean`
removes the containers it left behind.

CI runs the same lint, test, docs and acceptance steps. The docs job fails if
`make generate` changes anything, so commit `docs/` after changing a schema,
an example or a template.

Using a local build
-------------------

Run `make build` and point Terraform at `./build` with `dev_overrides` in
`~/.terraformrc`, or the file named by `TF_CLI_CONFIG_FILE`. `make terraformrc`
writes one for this checkout. Skip `terraform init` for overridden providers.

```hcl
provider_installation {
  dev_overrides {
    "ilijamt/technitium" = "/path/to/terraform-provider-technitium/build"
  }
  direct {}
}
```

Moving to a new Technitium release
----------------------------------

`technitium.Version` in
[internal/technitium/version.go](internal/technitium/version.go) is the release
the resources follow and the image `make testacc` runs.

1. Diff `APIDOCS.md` between the two tags, for example
   `https://github.com/TechnitiumSoftware/DnsServer/compare/v15.6.1...<new>`.
   `APIDOCS.md` leaves out some calls (LDAP is only in the code), so also
   compare the routes the server registers in
   `DnsServerCore/DnsWebService.cs`.
2. Update the specs in `internal/technitium` for the parameters that changed.
3. Bump `Version`, then run `make generate` and `make testacc`.
