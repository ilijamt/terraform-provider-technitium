resource "technitium_zone" "test" {
  zone = "nsec3.test"
}

resource "technitium_zone_dnssec" "test" {
  zone              = technitium_zone.test.zone
  algorithm         = "ECDSA"
  curve             = "P256"
  nx_proof          = "NSEC3"
  nsec3_iterations  = 1
  nsec3_salt_length = 8
}
