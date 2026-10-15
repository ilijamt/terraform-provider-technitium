resource "technitium_zone" "ptr_home" {
  zone = "example.com"
}

resource "technitium_zone" "ptr_reverse" {
  zone = "178.168.192.in-addr.arpa"
}

resource "technitium_record_a" "ptr_nas" {
  zone       = technitium_zone.ptr_home.zone
  domain     = "nas.example.com"
  ip_address = "192.168.178.10"
}

resource "technitium_record_ptr" "nas" {
  zone     = technitium_zone.ptr_reverse.zone
  domain   = "10.178.168.192.in-addr.arpa"
  ptr_name = technitium_record_a.ptr_nas.domain

  # Deleting or replacing the A record makes the server delete this PTR too.
  # This writes it again in the same apply.
  lifecycle {
    replace_triggered_by = [technitium_record_a.ptr_nas]
  }
}
