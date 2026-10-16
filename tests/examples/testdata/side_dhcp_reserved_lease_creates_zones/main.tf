resource "technitium_dhcp_scope" "side" {
  name             = "side107"
  starting_address = "192.168.107.100"
  ending_address   = "192.168.107.200"
  subnet_mask      = "255.255.255.0"
  domain_name      = "side107.test"
  dns_updates      = true
}

resource "technitium_dhcp_reserved_lease" "nas" {
  scope            = technitium_dhcp_scope.side.name
  hardware_address = "00-11-22-00-01-07"
  ip_address       = "192.168.107.10"
  host_name        = "nas"
}
