resource "technitium_record_fwd" "lan" {
  zone      = "lan.example.com"
  domain    = "lan.example.com"
  protocol  = "Udp"
  forwarder = "192.168.178.2"
}
