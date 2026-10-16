resource "technitium_zone" "test" {
  zone = "ttl.test"
}

resource "technitium_record_a" "a" {
  zone       = technitium_zone.test.zone
  domain     = "www.ttl.test"
  ip_address = "192.0.2.1"
  ttl        = 60
}

resource "technitium_record_a" "b" {
  zone       = technitium_zone.test.zone
  domain     = "www.ttl.test"
  ip_address = "192.0.2.2"
  ttl        = 60
  comments   = "keep"
  expiry_ttl = 86400
  disable    = true
  depends_on = [technitium_record_a.a]
}
