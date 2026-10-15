resource "technitium_zone" "ptr_reverse" {
  zone = "178.168.192.in-addr.arpa"
}

resource "technitium_record_ptr" "nas" {
  zone     = technitium_zone.ptr_reverse.zone
  domain   = "10.178.168.192.in-addr.arpa"
  ptr_name = "nas.example.com"
}
