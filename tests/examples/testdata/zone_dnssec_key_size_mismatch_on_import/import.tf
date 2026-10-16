resource "technitium_zone" "test" {
  zone = "rsasize.test"
}

import {
  to = technitium_zone_dnssec.test
  id = "rsasize.test"
}

resource "technitium_zone_dnssec" "test" {
  zone           = technitium_zone.test.zone
  algorithm      = "RSA"
  hash_algorithm = "SHA256"
  ksk_key_size   = 2048
  zsk_key_size   = 2048
}
