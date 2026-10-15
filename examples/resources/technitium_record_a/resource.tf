resource "technitium_zone" "a_home" {
  zone = "home.example.com"
}

resource "technitium_zone" "a_reverse" {
  zone = "178.168.192.in-addr.arpa"
}

# ip_address cannot be "request-ip-address", which only the APP record type uses.
resource "technitium_record_a" "nas" {
  zone       = technitium_zone.a_home.zone
  domain     = "nas.home.example.com"
  ip_address = "192.168.178.10"
  ttl        = 300
}

# Deleting or replacing the A record makes the server delete its PTR, so the
# PTR is replaced with it to write it again in the same apply.
resource "technitium_record_ptr" "nas" {
  zone     = technitium_zone.a_reverse.zone
  domain   = "10.178.168.192.in-addr.arpa"
  ptr_name = technitium_record_a.nas.domain

  lifecycle {
    replace_triggered_by = [technitium_record_a.nas]
  }
}
