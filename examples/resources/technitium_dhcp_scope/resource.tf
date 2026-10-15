# With dns_updates the server creates the zones leases need when missing,
# so create them before the scope.
resource "technitium_zone" "home" {
  zone = "home.example.com"
}

resource "technitium_zone" "home_reverse" {
  zone = "1.168.192.in-addr.arpa"
}

resource "technitium_dhcp_scope" "lan" {
  name                = "lan"
  starting_address    = "192.168.1.100"
  ending_address      = "192.168.1.250"
  subnet_mask         = "255.255.255.0"
  router_address      = "192.168.1.1"
  use_this_dns_server = true
  domain_name         = technitium_zone.home.zone
  dns_updates         = true

  exclusions = [{
    starting_address = "192.168.1.100"
    ending_address   = "192.168.1.109"
  }]

  depends_on = [technitium_zone.home_reverse]
}
