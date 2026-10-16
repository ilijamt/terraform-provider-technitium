# Integration tests

End-to-end tests that apply Terraform configs from files against a real
Technitium DNS Server. The run starts a `technitium/dns-server` container of
the version in `technitium.Version`, runs every test against it, and removes
it.

## Layout

```
tests/
  README.md
  examples/
    harness_test.go           # TestMain, provider factory and fixtures
    helpers_test.go           # shared helpers for the tests
    <area>_test.go            # TestIntegration_* tests
    testdata/
      <name>/main.tf          # first step
      <name>/update.tf        # later steps, one file each
```

Every file in `tests/examples` has the `integration` build tag, so
`go test ./...` and `make test` skip them and need no Docker.

## Running

```sh
make test-integration
```

This needs Docker. `TestMain` starts one container, creates an API token on
it, and passes the address and token to the provider in a `provider` block.
The container is removed when the run ends, also on Ctrl-C. A run that is
killed outright can leave it behind; `make testacc-clean` removes containers
with the label `terraform-provider-technitium.testacc`.

To run one test:

```sh
TF_ACC=1 go test -tags=integration ./tests/... -run '^TestIntegration_ZoneDNSSEC$' -count=1 -v
```

## OpenTofu

```sh
make test-integration TF=tofu
```

`TF=tofu` sets `TF_ACC_TERRAFORM_PATH` and `TF_ACC_PROVIDER_HOST` together.
OpenTofu resolves `ilijamt/technitium` to `registry.opentofu.org` and rejects
the harness default under `registry.terraform.io`. When you run `go test` by
hand with only `TF_ACC_TERRAFORM_PATH` pointing at `tofu`, `TestMain` sets the
provider host itself.

For coverage, `make test-integration-cover` writes into `COVERDIR`:

```sh
make test-integration-cover TF=tofu COVERDIR=build/covdata-tofu
```

## Adding a test

1. Put each step's config in `tests/examples/testdata/<name>/`, starting with
   `main.tf`, then `update.tf` and further files named after what the step
   does. Leave out `terraform { required_providers }` and the `provider`
   block. Keep configs static; prefer one more file over a Go template.
2. Add `TestIntegration_<Name>` to the `<area>_test.go` file it fits, or a new
   one, with `//go:build integration` at the top. Read each step with
   `ReadFixture(t, "<name>", "main.tf")`, which puts the provider block in
   front.
3. Use `factories`, and `zonesDestroyed` for `CheckDestroy`. Make
   out-of-band calls with `api()`, `call` and `records`, or `callWith` for
   another client. `expectCreate`, `expectUpdate`, `expectReplace` and
   `expectNoop` give the plan checks.
4. A test that needs a newer Terraform or OpenTofu calls `skipBelow`, which
   skips it with the reason.

Fixtures. A value only known at run time, such as a second server's IP,
goes in a `variable` block in the fixture and in `ConfigVariables` on the
step. For an export, pass `Host: host, Token: token` to
`terraforming.Options`, and put `providerHeader()` in front of what
`readConfig` returns.

Names. All tests share one server, so a zone, user, group or scope name must
be unique across the package, not only in its file. Search `tests/examples`
before you pick one.

Settings. A test that changes server settings puts them back in
`t.Cleanup`, or uses `resetSSO` and `resetLDAP`. Register the cleanup before
the first change, so a failed step still restores them.

A second server. `ownTestServerWith(t, ports...)` starts a server of the
test's own and returns its host, its IP on the Docker network and a client
signed in to it. The shared server reaches it on that IP, as a primary for
secondary zones. `ownTestServer(t)` returns only the host and client. To
apply a config against it, read the fixture with
`ReadFixtureFor(t, host, name, file)`. The container is removed when the
test ends. Use `freePort(t)` for a port to publish.

Docker exec. `dockerExec(t, args...)` runs a command in the shared server's
container, as in `dockerExec(t, "cat", "/etc/dns/dns.config")`. For a
server of the test's own, `ownContainer(t)` returns the container, and
`dockerExecIn(t, s.ID, args...)` runs in it.

Restarts. A test that restarts the web or DNS service, or changes the web
service port or addresses, runs on its own server and goes in its own file,
such as `settings_restart_test.go`. It must never restart the shared
server, since every later test would fail while it comes back.

## Notes

- The tests never read `TECHNITIUM_HOST` or `TECHNITIUM_TOKEN`. `TestMain`
  unsets them, and the provider block sets every value, so the provider
  cannot fall back to a server you manage.
- `TestMain` always starts its own container. There is no option to run
  against an external server.
- All tests share one server, so each one uses its own zone names and
  removes what it creates. A test that changes server settings must put them
  back.
