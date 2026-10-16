resource "technitium_zone" "acl" {
  zone                     = "acl.test"
  query_access             = "UseSpecifiedNetworkACL"
  query_access_network_acl = ["10.0.0.0/8", "!192.168.1.1"]
  notify                   = "SpecifiedNameServers"
  notify_name_servers      = ["192.0.2.1", "192.0.2.2"]
}
