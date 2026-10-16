resource "technitium_zone" "test" {
  zone = "ipv6.test"
}

resource "technitium_record_aaaa" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.ipv6.test"
  ip_address = "2001:DB8:0::1"
}

resource "technitium_record_a" "me" {
  zone       = technitium_zone.test.zone
  domain     = "me.ipv6.test"
  ip_address = "request-ip-address"
}
