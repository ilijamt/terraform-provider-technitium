resource "technitium_zone" "ds_home" {
  zone = "ds.example.com"
}

resource "technitium_record_ds" "child" {
  zone        = technitium_zone.ds_home.zone
  domain      = "child.ds.example.com"
  key_tag     = 12345
  algorithm   = "ECDSAP256SHA256"
  digest_type = "SHA256"
  digest      = "49FD46E6C4B45C55D4AC49FD46E6C4B45C55D4AC49FD46E6C4B45C55D4AC1234"
}
