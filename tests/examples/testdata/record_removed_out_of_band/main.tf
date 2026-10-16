resource "technitium_zone" "test" {
  zone = "gone.test"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.gone.test"
  ip_address = "10.0.0.1"
}
