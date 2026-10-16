resource "technitium_zone" "reverse" {
  zone = "178.168.192.in-addr.arpa"
  type = "Forwarder"
}

resource "technitium_record_fwd" "reverse" {
  zone      = technitium_zone.reverse.zone
  domain    = technitium_zone.reverse.zone
  protocol  = "Udp"
  forwarder = "192.168.178.2"
}
