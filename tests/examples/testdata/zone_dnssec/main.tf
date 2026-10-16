resource "technitium_zone" "test" {
  zone = "signed.test"
}

resource "technitium_zone_dnssec" "test" {
  zone              = technitium_zone.test.zone
  algorithm         = "ECDSA"
  curve             = "P256"
  nx_proof          = "NSEC3"
  zsk_rollover_days = 30
}

data "technitium_zone_dnssec" "test" {
  zone       = technitium_zone_dnssec.test.zone
  depends_on = [technitium_zone_dnssec.test]
}
