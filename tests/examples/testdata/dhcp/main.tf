resource "technitium_dhcp_scope" "lan" {
  name             = "lan"
  starting_address = "192.168.50.10"
  ending_address   = "192.168.50.200"
  subnet_mask      = "255.255.255.0"
  router_address   = "192.168.50.1"
  dns_servers      = ["192.168.50.2", "192.168.50.3"]
  domain_name      = "lan.test"
}

# The server writes MAC addresses with dashes in upper case.
resource "technitium_dhcp_reserved_lease" "printer" {
  scope            = technitium_dhcp_scope.lan.name
  hardware_address = "00:11:22:aa:bb:cc"
  ip_address       = "192.168.50.100"
  host_name        = "printer"
  comments         = "office"
}
