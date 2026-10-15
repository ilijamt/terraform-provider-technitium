resource "technitium_zone" "uri_home" {
  zone = "uri.example.com"
}

resource "technitium_record_uri" "sip" {
  zone         = technitium_zone.uri_home.zone
  domain       = "_sip._tcp.uri.example.com"
  uri_priority = 10
  uri_weight   = 1
  uri          = "https://sip.uri.example.com/"
}
