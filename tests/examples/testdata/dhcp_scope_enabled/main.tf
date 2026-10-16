# A new scope starts enabled, so create disables it.
resource "technitium_dhcp_scope" "toggle" {
  name             = "toggle"
  starting_address = "192.168.90.10"
  ending_address   = "192.168.90.20"
  subnet_mask      = "255.255.255.0"
  enabled          = false
}

data "technitium_dhcp_scope" "toggle" {
  name       = technitium_dhcp_scope.toggle.name
  depends_on = [technitium_dhcp_scope.toggle]
}
