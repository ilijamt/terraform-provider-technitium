resource "technitium_zone" "test" {
  zone = "ptr.test"
}

resource "technitium_zone" "reverse" {
  zone = "100.51.198.in-addr.arpa"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.ptr.test"
  ip_address = "198.51.100.7"
  ttl        = 600
}

resource "technitium_record_ptr" "www" {
  zone     = technitium_zone.reverse.zone
  domain   = "7.100.51.198.in-addr.arpa"
  ptr_name = technitium_record_a.www.domain

  lifecycle {
    replace_triggered_by = [technitium_record_a.www]
  }
}
