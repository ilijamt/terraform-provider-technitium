resource "technitium_dhcp_scope" "taken" {
  name             = "taken"
  starting_address = "192.168.60.100"
  ending_address   = "192.168.60.200"
  subnet_mask      = "255.255.255.0"
}
