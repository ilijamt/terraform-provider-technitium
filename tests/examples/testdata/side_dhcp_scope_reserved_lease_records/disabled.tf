resource "technitium_zone" "fwd" {
  zone = "side115.test"
}

resource "technitium_zone" "rev" {
  zone = "115.168.192.in-addr.arpa"
}

resource "technitium_dhcp_scope" "side" {
  name             = "side115"
  starting_address = "192.168.115.100"
  ending_address   = "192.168.115.200"
  subnet_mask      = "255.255.255.0"
  domain_name      = technitium_zone.fwd.zone
  dns_updates      = true
  enabled          = false
  depends_on       = [technitium_zone.rev]
}

resource "technitium_dhcp_reserved_lease" "nas" {
  scope            = technitium_dhcp_scope.side.name
  hardware_address = "00-11-22-00-01-15"
  ip_address       = "192.168.115.10"
  host_name        = "nas"
}
