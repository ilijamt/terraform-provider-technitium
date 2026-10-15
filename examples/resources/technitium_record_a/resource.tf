resource "technitium_zone" "a_home" {
  zone = "home.example.com"
}

# ip_address cannot be "request-ip-address", which only the APP record type uses.
resource "technitium_record_a" "nas" {
  zone       = technitium_zone.a_home.zone
  domain     = "nas.home.example.com"
  ip_address = "192.168.178.10"
  ttl        = 300
  ptr        = true
}
