resource "technitium_dhcp_scope" "lan" {
  name             = "lan"
  starting_address = "192.168.50.10"
  ending_address   = "192.168.50.200"
  subnet_mask      = "255.255.255.0"
  router_address   = "192.168.50.1"
  dns_servers      = ["192.168.50.2", "192.168.50.3"]
  domain_name      = "lan.test"

  exclusions      = [{ starting_address = "192.168.50.10", ending_address = "192.168.50.20" }]
  static_routes   = [{ destination = "172.16.0.0", subnet_mask = "255.255.0.0", router = "192.168.50.254" }]
  generic_options = [{ code = 150, value = "C0:A8:32:05" }]
  vendor_info     = [{ identifier = "PXEClient", information = "06:01:03" }]
}

resource "technitium_dhcp_reserved_lease" "printer" {
  scope            = technitium_dhcp_scope.lan.name
  hardware_address = "00-11-22-AA-BB-CC"
  ip_address       = "192.168.50.100"
  host_name        = "printer"
  comments         = "office"
}
