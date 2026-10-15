resource "technitium_zone" "rp_home" {
  zone = "rp.example.com"
}

resource "technitium_record_rp" "owner" {
  zone       = technitium_zone.rp_home.zone
  domain     = technitium_zone.rp_home.zone
  mailbox    = "hostmaster@rp.example.com"
  txt_domain = "info.rp.example.com"
}
