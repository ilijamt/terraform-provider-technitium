Technitium Terraform Provider
=============================

A Terraform provider for [Technitium DNS Server](https://technitium.com/dns/),
built on its
[HTTP API](https://github.com/TechnitiumSoftware/DnsServer/blob/master/APIDOCS.md).
It supports Technitium 15.6.1 and Terraform 1.5 or later, tested with 1.5.7
and 1.16.3.

```hcl
terraform {
  required_providers {
    technitium = {
      source = "ilijamt/technitium"
    }
  }
}

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

What it manages
---------------

- Zones, their `SOA` record and DNSSEC signing.
- Records, with one `technitium_record_<type>` per type: A, AAAA, NS, CNAME,
  PTR, MX, TXT, RP, SRV, NAPTR, DNAME, DS, SSHFP, TLSA, SVCB, HTTPS, URI, CAA,
  ANAME, FWD and APP.
- DHCP scopes and reserved leases.
- Allowed and blocked domains.
- DNS apps and their config. The `technitium_store_app` data source gives an
  app's download URL in the DNS App Store.
- API tokens for other tools.
- Users, groups and group members, with one `technitium_permissions_<section>`
  per server section and `technitium_zone_permissions` per zone.
- Settings, with one `technitium_settings_<area>` per tab of the Settings
  page, plus SSO and LDAP authentication. Each one manages only its own tab.

Most resources have a data source that reads the object without managing it.

Documentation
-------------

- [Resources and data sources](docs/)
- [Exporting an existing server](docs/guides/exporting.md) with
  `tf-technitium`, which writes the configuration and import blocks for you
- [Development](CONTRIBUTING.md): building, testing and moving to a new
  Technitium release
