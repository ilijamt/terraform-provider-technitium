resource "technitium_zone" "fwd_lan" {
  zone = "lan.example.com"
  type = "Forwarder"
}

# The server keeps every FWD record at TTL 0, so there is no ttl.
resource "technitium_record_fwd" "lan" {
  zone      = technitium_zone.fwd_lan.zone
  domain    = technitium_zone.fwd_lan.zone
  protocol  = "Udp"
  forwarder = "192.168.178.2"
}
