resource "technitium_zone" "test" {
  zone = "fwdttl.test"
  type = "Forwarder"
}

resource "technitium_record_fwd" "test" {
  zone      = technitium_zone.test.zone
  domain    = "sub.fwdttl.test"
  forwarder = "192.0.2.53"
}
