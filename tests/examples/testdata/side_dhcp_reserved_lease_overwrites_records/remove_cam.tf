resource "technitium_zone" "fwd" {
  zone = "side117.test"
}

resource "technitium_zone" "rev" {
  zone = "117.168.192.in-addr.arpa"
}

resource "technitium_dhcp_scope" "side" {
  name             = "side117"
  starting_address = "192.168.117.100"
  ending_address   = "192.168.117.200"
  subnet_mask      = "255.255.255.0"
  domain_name      = technitium_zone.fwd.zone
  dns_updates      = true
  dns_ttl          = 3600
  depends_on       = [technitium_zone.rev]
}

resource "technitium_record_a" "nas" {
  zone       = technitium_zone.fwd.zone
  domain     = "nas.side117.test"
  ip_address = "192.168.117.50"
  ttl        = 3600
}

resource "technitium_record_a" "cam" {
  zone       = technitium_zone.fwd.zone
  domain     = "cam.side117.test"
  ip_address = "192.168.117.11"
  ttl        = 3600
}

resource "technitium_dhcp_reserved_lease" "nas" {
  scope            = technitium_dhcp_scope.side.name
  hardware_address = "00-11-22-00-01-10"
  ip_address       = "192.168.117.10"
  host_name        = "nas"
}
