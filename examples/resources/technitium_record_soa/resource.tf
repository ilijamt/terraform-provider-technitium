resource "technitium_record_soa" "home" {
  zone                   = technitium_zone.home.zone
  domain                 = technitium_zone.home.zone
  responsible_person     = "hostmaster@home.example.com"
  use_serial_date_scheme = true
}
