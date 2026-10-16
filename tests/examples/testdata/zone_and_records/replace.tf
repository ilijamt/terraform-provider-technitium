resource "technitium_zone" "test" {
  zone = "records.test"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.records.test"
  ip_address = "10.0.0.2"
  ttl        = 300
}

resource "technitium_record_cname" "app" {
  zone   = technitium_zone.test.zone
  domain = "app.records.test"
  cname  = technitium_record_a.www.domain
}
