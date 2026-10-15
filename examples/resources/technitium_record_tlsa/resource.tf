resource "technitium_zone" "tlsa_home" {
  zone = "tlsa.example.com"
}

resource "technitium_record_tlsa" "https" {
  zone                              = technitium_zone.tlsa_home.zone
  domain                            = "_443._tcp.tlsa.example.com"
  tlsa_certificate_usage            = "DANE-EE"
  tlsa_selector                     = "SPKI"
  tlsa_matching_type                = "SHA2-256"
  tlsa_certificate_association_data = "D4A7E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852"
}
