resource "technitium_zone_dnssec" "home" {
  zone      = technitium_zone.home.zone
  algorithm = "ECDSA"
  curve     = "P256"
  nx_proof  = "NSEC3"

  # Replacing this resource signs the zone with new keys, which breaks
  # validation until the parent zone has the new DS records.
  lifecycle {
    prevent_destroy = true
  }
}

# The DS records to register at the parent zone.
output "ds_records" {
  value = technitium_zone_dnssec.home.ds_records
}
