resource "technitium_zone" "srv_home" {
  zone = "srv.example.com"
}

resource "technitium_record_srv" "sip" {
  zone     = technitium_zone.srv_home.zone
  domain   = "_sip._tcp.srv.example.com"
  priority = 10
  weight   = 60
  port     = 5060
  target   = "sip.srv.example.com"
}
