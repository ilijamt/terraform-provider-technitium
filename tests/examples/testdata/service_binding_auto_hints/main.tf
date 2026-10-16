resource "technitium_zone" "test" {
  zone = "hints.test"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.hints.test"
  ip_address = "10.0.0.1"
}

resource "technitium_record_https" "www" {
  zone            = technitium_zone.test.zone
  domain          = "www.hints.test"
  svc_priority    = 1
  svc_target_name = "www.hints.test"
  svc_params      = { alpn = "h2" }
  auto_ipv4_hint  = true
  ttl             = 300
  depends_on      = [technitium_record_a.www]
}
