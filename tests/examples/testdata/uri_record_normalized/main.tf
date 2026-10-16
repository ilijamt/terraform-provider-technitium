resource "technitium_zone" "test" {
  zone = "uri.test"
}

resource "technitium_record_uri" "web" {
  zone         = technitium_zone.test.zone
  domain       = "_http._tcp.uri.test"
  uri_priority = 10
  uri_weight   = 1
  uri          = "HTTPS://Example.com"
}
