resource "technitium_zone" "test" {
  zone = "drift-oob.test"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.drift-oob.test"
  ip_address = "10.0.0.1"
  ttl        = 300
  comments   = "managed"
}
