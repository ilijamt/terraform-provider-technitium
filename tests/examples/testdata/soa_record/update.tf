resource "technitium_zone" "test" {
  zone = "soa.test"
}

resource "technitium_record_soa" "test" {
  zone    = technitium_zone.test.zone
  domain  = technitium_zone.test.zone
  refresh = 7200
  minimum = 600
}

# Adding a record bumps the serial, which must not show as drift.
resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.soa.test"
  ip_address = "10.0.0.1"
  depends_on = [technitium_record_soa.test]
}
