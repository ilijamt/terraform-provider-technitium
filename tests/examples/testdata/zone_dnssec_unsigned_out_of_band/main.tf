resource "technitium_zone" "test" {
  zone = "unsigned-oob.test"
}

resource "technitium_zone_dnssec" "test" {
  zone      = technitium_zone.test.zone
  algorithm = "ECDSA"
  curve     = "P256"
  nx_proof  = "NSEC3"
}
