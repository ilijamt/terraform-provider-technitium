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

- `technitium_zone`, and `technitium_record_soa` for the `SOA` record every zone has, and
  `technitium_zone_dnssec` to sign a zone with DNSSEC
- one `technitium_record_<type>` per record type: A, AAAA, NS, CNAME, PTR, MX,
  TXT, RP, SRV, NAPTR, DNAME, DS, SSHFP, TLSA, SVCB, HTTPS, URI, CAA, ANAME, FWD
  and APP
- `technitium_dhcp_scope`, and `technitium_dhcp_reserved_lease` for each reserved address
- `technitium_allowed_zone` and `technitium_blocked_zone` for domains to allow or block
- `technitium_app` to install DNS apps and `technitium_app_config` for their config, with
  `data "technitium_store_app"` for an app's download URL in the DNS App Store
- `technitium_api_token` to issue API tokens for other tools
- `technitium_user`, `technitium_group`, `technitium_group_members`, and one
  `technitium_permissions_<section>` per section of the server (dashboard,
  zones, cache, allowed, blocked, apps, dns_client, settings, dhcp_server,
  administration, logs) and `technitium_zone_permissions` per zone, for access
  to the web console and API
- one `technitium_settings_<area>` per tab of the Settings page: general,
  web_service, optional_protocols, tsig, recursion, cache, blocking,
  proxy_forwarders and logging. Each manages only its own settings.
- `technitium_settings_sso` for single sign-on: the OpenID Connect provider,
  sign-up rules and the group map, and `technitium_settings_ldap` for LDAP
  authentication

Data sources read what a resource manages without managing it, found by the
same identity: zones and their `SOA`, settings, DHCP scopes and reserved
leases, users, groups (the built-in ones included), group members and
permissions.

See [docs](docs/).

Exporting a server
------------------

`tf-technitium` writes Terraform configuration for everything on a server, like
[cf-terraforming](https://github.com/cloudflare/cf-terraforming) does for
Cloudflare: a file of resources per group (`zones.tf` with each zone and its
`SOA`, `settings.tf`, `dhcp.tf`, `blocking.tf`, `access.tf`, `apps.tf`), a
`records_<zone>.tf` per zone, `imports.tf` with an `import` block for each, and
`variables.tf` for the secrets it does not write out. Resources refer to each
other (`zone = technitium_zone.home_example_com.zone`) instead of repeating
names, and import IDs use the short `zone/domain/value` form unless a value
needs JSON. The next `terraform apply` (Terraform 1.5 or later) adopts
everything. SSO users, whose accounts belong to the SSO provider, are written as
`data "technitium_user"` blocks, like the built-in groups.

```shell
make build
export TECHNITIUM_HOST=http://dns.example.com:5380 TECHNITIUM_TOKEN=...
./build/tf-technitium -out ./dns                      # everything
./build/tf-technitium -out ./dns -zone home.example.com -type 'zone,record_*'
```

Every value is read through the provider, so the plan after the export shows
no changes. The exceptions are the secrets: user passwords, which the server
never returns, and TSIG shared secrets become sensitive variables, and the first
apply sets each user's password to its variable. Exported users get
`prevent_destroy`, so an existing account like `admin` is never deleted by
accident. The built-in groups, which cannot be deleted or renamed, are written
as `data "technitium_group"` blocks that the group members and permissions refer
to. Permissions are written in full, `user_permissions = []` included, so
Terraform owns each section's lists.

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
   `APIDOCS.md` misses some calls (LDAP is only in the code), so also compare
   the routes the server registers in `DnsServerCore/DnsWebService.cs`.
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
