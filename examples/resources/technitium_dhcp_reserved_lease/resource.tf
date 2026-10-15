resource "technitium_dhcp_scope" "lease_lan" {
  name             = "lease-lan"
  starting_address = "192.168.1.100"
  ending_address   = "192.168.1.250"
  subnet_mask      = "255.255.255.0"
}

resource "technitium_dhcp_reserved_lease" "nas" {
  scope            = technitium_dhcp_scope.lease_lan.name
  hardware_address = "00-11-22-AA-BB-CC"
  ip_address       = "192.168.1.10"
  host_name        = "nas"
}
