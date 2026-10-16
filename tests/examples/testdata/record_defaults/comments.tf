resource "technitium_zone" "test" {
  zone = "defaults.test"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.defaults.test"
  ip_address = "10.0.0.1"
  comments   = "web"
}
