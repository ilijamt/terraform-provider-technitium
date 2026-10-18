Development
===========

```shell
make build            # ./build/terraform-provider-technitium and ./build/tf-technitium
make generate         # render docs/ from the schemas, examples/ and templates/
make test             # unit tests, no Docker
make test-integration # tests/examples in a container; TF=tofu for OpenTofu
make lint             # gofmt, terraform fmt, go vet and golangci-lint
```

`make test-integration` needs Docker. It starts one container of the image
named by `technitium.Version`, runs every test in `tests/examples` against
it, and removes it. The tests create and delete zones and change settings,
so they ignore `TECHNITIUM_HOST` and `TECHNITIUM_TOKEN` and never touch the
server you manage. If a run is killed, `make testacc-clean` removes the
containers it left behind. See [tests/README.md](tests/README.md) for
running one test, coverage and adding a test.

`make test-integration TF=tofu` runs them with OpenTofu. It sets
`TF_ACC_TERRAFORM_PATH` to `tofu` and `TF_ACC_PROVIDER_HOST` to
`registry.opentofu.org`, because OpenTofu resolves `ilijamt/technitium` there.

To test another version, point `TF_ACC_TERRAFORM_PATH` at its binary, for
example `TF_ACC_TERRAFORM_PATH=/path/to/terraform-1.5.7 make test-integration`.
When that path runs OpenTofu, the tests set the provider host themselves. Run the oldest supported
versions (Terraform 1.5, OpenTofu 1.6) and the newest before a release.
Tests that need a newer version skip with the reason.

Run `make generate` and commit `docs/` after changing a schema, an example
or a template.

Using a local build
-------------------

Run `make build` and point Terraform at `./build` with `dev_overrides` in
`~/.terraformrc`, or the file named by `TF_CLI_CONFIG_FILE`. OpenTofu reads
`~/.tofurc`, or `~/.terraformrc` when there is no `~/.tofurc`, and also honors
`TF_CLI_CONFIG_FILE`. `make terraformrc` writes one for this checkout. Skip
`init` for overridden providers.

```hcl
provider_installation {
  dev_overrides {
    "ilijamt/technitium" = "/path/to/terraform-provider-technitium/build"
  }
  direct {}
}
```

To attach a debugger, run the provider with `-debug` and export the
`TF_REATTACH_PROVIDERS` it prints. With OpenTofu, run it with
`-debug -address registry.opentofu.org/ilijamt/technitium`.

Moving to a new Technitium release
----------------------------------

`technitium.Version` in
[internal/technitium/version.go](internal/technitium/version.go) is the release
the resources follow and the image `make test-integration` runs.

1. Diff `APIDOCS.md` between the two tags, for example
   `https://github.com/TechnitiumSoftware/DnsServer/compare/v15.6.2...<new>`.
   `APIDOCS.md` leaves out some calls (LDAP is only in the code), so also
   compare the routes the server registers in
   `DnsServerCore/DnsWebService.cs`.
2. Update the specs in `internal/technitium` for the parameters that changed.
3. Bump `Version`, then run `make generate` and `make test-integration`.
