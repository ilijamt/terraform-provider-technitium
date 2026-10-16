resource "technitium_zone" "test" {
  zone = "nsttl.test"
}

resource "technitium_record_ns" "sub" {
  zone        = technitium_zone.test.zone
  domain      = "sub.nsttl.test"
  name_server = "ns1.example.net"
}
