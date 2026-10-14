Technitium Terraform Provider
=============================

A Terraform provider for [Technitium DNS Server](https://technitium.com/dns/),
written against its
[HTTP API](https://github.com/TechnitiumSoftware/DnsServer/blob/master/APIDOCS.md).

```hcl
provider "technitium" {
  host  = "http://dns.example.com:5380"
  token = var.technitium_token # or username/password, or TECHNITIUM_* env vars
}

resource "technitium_zone" "home" {
  zone = "home.example.com"
}

resource "technitium_record_a" "nas" {
  zone       = technitium_zone.home.zone
  domain     = "nas.home.example.com"
  ip_address = "192.168.178.10"
}
```

Resources:

- `technitium_zone`, and `technitium_record_soa` for the `SOA` record every zone has
- one `technitium_record_<type>` per record type: A, AAAA, NS, CNAME, PTR, MX,
  TXT, RP, SRV, NAPTR, DNAME, DS, SSHFP, TLSA, SVCB, HTTPS, URI, CAA, ANAME, FWD
  and APP
- `technitium_dhcp_scope`, and `technitium_dhcp_reserved_lease` for each reserved address
- `technitium_allowed_zone` and `technitium_blocked_zone` for domains to allow or block
- `technitium_user`, `technitium_group` and `technitium_permissions` for access to the web console and API
- one `technitium_settings_<area>` per tab of the Settings page: general,
  web_service, optional_protocols, tsig, recursion, cache, blocking,
  proxy_forwarders and logging. Each manages only its own settings.

See [docs](docs/).

How it works
------------

Each resource is a `framework.Spec` written by hand in
[internal/technitium](internal/technitium): the API calls it makes and, per
attribute, the parameter it sends and where the read response holds it. A
single generic resource in [internal/framework](internal/framework) serves
every spec. Records use `framework.Record`, which adds the attributes every
record type shares.

Upgrading Technitium
--------------------

`technitium.Version` in
[internal/technitium/version.go](internal/technitium/version.go) names the
release the resources follow and the image `make testacc` runs. To move to a
new release:

1. Diff `APIDOCS.md` between the two tags, for example
   `https://github.com/TechnitiumSoftware/DnsServer/compare/v15.6.1...<new>`.
2. Update the specs for the parameters that changed.
3. Bump `Version`, then run `make generate` and `make testacc`.

Development
-----------

```shell
make generate   # render the docs
make build      # build ./build/terraform-provider-technitium
make test       # unit tests
make testacc    # Terraform runs against a technitium/dns-server container (Docker)
```
