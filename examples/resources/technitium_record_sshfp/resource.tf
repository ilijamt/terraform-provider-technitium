resource "technitium_zone" "sshfp_home" {
  zone = "sshfp.example.com"
}

resource "technitium_record_sshfp" "host" {
  zone                   = technitium_zone.sshfp_home.zone
  domain                 = "host.sshfp.example.com"
  sshfp_algorithm        = "Ed25519"
  sshfp_fingerprint_type = "SHA256"
  sshfp_fingerprint      = "D4A7E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852"
}
