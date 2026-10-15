data "technitium_dhcp_scope" "lan" {
  name = "lan"
}

output "lan_router" {
  value = data.technitium_dhcp_scope.lan.router_address
}
