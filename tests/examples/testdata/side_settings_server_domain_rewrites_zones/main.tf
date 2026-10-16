resource "technitium_zone" "side" {
  zone = "side108.test"
}

resource "technitium_record_soa" "side" {
  zone                   = technitium_zone.side.zone
  domain                 = technitium_zone.side.zone
  use_serial_date_scheme = true
}
