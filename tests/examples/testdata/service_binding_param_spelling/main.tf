resource "technitium_zone" "test" {
  zone = "svcparams.test"
}

resource "technitium_record_https" "www" {
  zone            = technitium_zone.test.zone
  domain          = "www.svcparams.test"
  svc_priority    = 1
  svc_target_name = "."
  svc_params = {
    ipv4hint          = "10.0.0.2, 10.0.0.1"
    ipv6hint          = "2001:DB8::1,2001:db8:0:0::2"
    port              = "0443"
    alpn              = "h2, h3"
    mandatory         = "ALPN,port"
    ech               = "aabb"
    "no-default-alpn" = ""
  }
  ttl = 300
}
