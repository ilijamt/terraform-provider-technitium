resource "technitium_zone" "ns_home" {
  zone = "ns.example.com"
}

resource "technitium_record_ns" "delegated" {
  zone        = technitium_zone.ns_home.zone
  domain      = "sub.ns.example.com"
  name_server = "ns1.sub.ns.example.com"
  glue        = "192.0.2.53"
}
