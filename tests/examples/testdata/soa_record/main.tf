resource "technitium_zone" "test" {
  zone = "soa.test"
}

resource "technitium_record_soa" "test" {
  zone    = technitium_zone.test.zone
  domain  = technitium_zone.test.zone
  refresh = 7200
  minimum = 300
}
