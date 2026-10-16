resource "technitium_zone" "test" {
  zone = "rsasize.test"
}

resource "technitium_zone_dnssec" "test" {
  zone         = technitium_zone.test.zone
  algorithm    = "ECDSA"
  curve        = "P256"
  zsk_key_size = 2048
}
