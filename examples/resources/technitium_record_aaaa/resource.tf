resource "technitium_zone" "aaaa_home" {
  zone = "aaaa.example.com"
}

resource "technitium_record_aaaa" "web" {
  zone       = technitium_zone.aaaa_home.zone
  domain     = "web.aaaa.example.com"
  ip_address = "2001:db8::10"
  ttl        = 300
}
