resource "technitium_zone" "nsec" {
  zone = "nsec-destroy.test"
}

resource "technitium_zone_dnssec" "nsec" {
  zone      = technitium_zone.nsec.zone
  algorithm = "ECDSA"
  curve     = "P256"
  nx_proof  = "NSEC"
}

resource "technitium_record_a" "nsec" {
  count      = 12
  zone       = technitium_zone.nsec.zone
  domain     = "host${count.index}.nsec-destroy.test"
  ip_address = "10.0.0.${count.index + 1}"
}

resource "technitium_zone" "nsec3" {
  zone = "nsec3-destroy.test"
}

resource "technitium_zone_dnssec" "nsec3" {
  zone      = technitium_zone.nsec3.zone
  algorithm = "ECDSA"
  curve     = "P256"
  nx_proof  = "NSEC3"
}

resource "technitium_record_a" "nsec3" {
  count      = 12
  zone       = technitium_zone.nsec3.zone
  domain     = "host${count.index}.nsec3-destroy.test"
  ip_address = "10.0.0.${count.index + 1}"
}
