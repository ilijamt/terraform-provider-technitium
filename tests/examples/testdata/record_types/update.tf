resource "technitium_zone" "types" {
  zone = "types.test"
}

resource "technitium_zone" "reverse" {
  zone = "2.0.192.in-addr.arpa"
}

resource "technitium_record_aaaa" "test" {
  zone       = technitium_zone.types.zone
  domain     = "v6.types.test"
  ip_address = "2001:db8::1"
  ttl        = 600
}

resource "technitium_record_aname" "test" {
  zone   = technitium_zone.types.zone
  domain = "alias.types.test"
  aname  = "target.example.net"
  ttl    = 600
}

resource "technitium_record_caa" "test" {
  zone   = technitium_zone.types.zone
  domain = "types.test"
  flags  = 0
  tag    = "issue"
  value  = "letsencrypt.org"
  ttl    = 600
}

resource "technitium_record_dname" "test" {
  zone   = technitium_zone.types.zone
  domain = "old.types.test"
  dname  = "new.example.net"
  ttl    = 600
}

resource "technitium_record_ds" "test" {
  zone        = technitium_zone.types.zone
  domain      = "child.types.test"
  key_tag     = 12345
  algorithm   = "ECDSAP256SHA256"
  digest_type = "SHA256"
  digest      = "2BB183AF5F22588179A53B0A98631FAD1A292118D3F04E8B8F6A1FB4A10A8D0D"
  ttl         = 600
}

resource "technitium_record_https" "test" {
  zone            = technitium_zone.types.zone
  domain          = "www.types.test"
  svc_priority    = 1
  svc_target_name = "www.types.test"
  svc_params      = { alpn = "h2,h3", port = "8443" }
  ttl             = 600
}

resource "technitium_record_mx" "test" {
  zone       = technitium_zone.types.zone
  domain     = "types.test"
  exchange   = "mail.types.test"
  preference = 10
  ttl        = 600
}

resource "technitium_record_ns" "test" {
  zone        = technitium_zone.types.zone
  domain      = "sub.types.test"
  name_server = "ns1.example.net"
  ttl         = 600
}

resource "technitium_record_ptr" "test" {
  zone     = technitium_zone.reverse.zone
  domain   = "1.2.0.192.in-addr.arpa"
  ptr_name = "host.types.test"
  ttl      = 600
}

resource "technitium_record_srv" "test" {
  zone     = technitium_zone.types.zone
  domain   = "_sip._tcp.types.test"
  priority = 10
  weight   = 5
  port     = 5060
  target   = "sip.types.test"
  ttl      = 600
}

resource "technitium_record_sshfp" "test" {
  zone                   = technitium_zone.types.zone
  domain                 = "host.types.test"
  sshfp_algorithm        = "Ed25519"
  sshfp_fingerprint_type = "SHA256"
  sshfp_fingerprint      = "A3B1A6F2B1E9D7C3F0E5A8B2C4D6E8F0A1B3C5D7E9F1A3B5C7D9E1F3A5B7C9D1"
  ttl                    = 600
}

resource "technitium_record_svcb" "test" {
  zone            = technitium_zone.types.zone
  domain          = "_dns.types.test"
  svc_priority    = 1
  svc_target_name = "dns.types.test"
  svc_params      = { alpn = "h2" }
  ttl             = 600
}

resource "technitium_record_tlsa" "test" {
  zone                              = technitium_zone.types.zone
  domain                            = "_443._tcp.types.test"
  tlsa_certificate_usage            = "DANE-EE"
  tlsa_selector                     = "SPKI"
  tlsa_matching_type                = "SHA2-256"
  tlsa_certificate_association_data = "0C72AC70B745AC19998811B131D662C9AC69DBDBE7CB23E5B514B56664C5D3D6"
  ttl                               = 600
}

resource "technitium_record_uri" "test" {
  zone         = technitium_zone.types.zone
  domain       = "_ftp._tcp.types.test"
  uri_priority = 10
  uri_weight   = 1
  uri          = "ftp://ftp.types.test/public"
  ttl          = 600
}
