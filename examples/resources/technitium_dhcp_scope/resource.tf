resource "technitium_dhcp_scope" "lan" {
  name                = "lan"
  starting_address    = "192.168.1.100"
  ending_address      = "192.168.1.250"
  subnet_mask         = "255.255.255.0"
  router_address      = "192.168.1.1"
  use_this_dns_server = true
  domain_name         = "home.example.com"
  dns_updates         = true

  exclusions = [{
    starting_address = "192.168.1.100"
    ending_address   = "192.168.1.109"
  }]
}
