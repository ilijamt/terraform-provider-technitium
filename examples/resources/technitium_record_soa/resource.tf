resource "technitium_zone" "soa_home" {
  zone = "home.example.com"
}

resource "technitium_record_soa" "home" {
  zone                   = technitium_zone.soa_home.zone
  domain                 = technitium_zone.soa_home.zone
  responsible_person     = "hostmaster@home.example.com"
  use_serial_date_scheme = true
}
