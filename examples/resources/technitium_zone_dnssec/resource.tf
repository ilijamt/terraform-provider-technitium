resource "technitium_zone" "dnssec_home" {
  zone = "home.example.com"
}

resource "technitium_zone_dnssec" "home" {
  zone      = technitium_zone.dnssec_home.zone
  algorithm = "ECDSA"
  curve     = "P256"
  nx_proof  = "NSEC3"

  # Replacing this resource signs the zone with new keys, which breaks
  # validation until the parent zone has the new DS records.
  lifecycle {
    prevent_destroy = true
  }
}

output "ds_records" {
  value = technitium_zone_dnssec.home.ds_records
}
