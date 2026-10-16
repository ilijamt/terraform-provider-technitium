resource "technitium_zone" "side" {
  zone = "side108.test"
}

resource "technitium_record_soa" "side" {
  zone                   = technitium_zone.side.zone
  domain                 = technitium_zone.side.zone
  use_serial_date_scheme = true
}

resource "technitium_settings_general" "this" {
  dns_server_domain = "ns.side108.test"
  depends_on        = [technitium_record_soa.side]
}
