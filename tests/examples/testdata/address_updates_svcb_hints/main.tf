resource "technitium_zone" "test" {
  zone = "hint.test"
}

resource "technitium_record_https" "www" {
  zone            = technitium_zone.test.zone
  domain          = "www.hint.test"
  svc_priority    = 1
  svc_target_name = "."
  svc_params      = { alpn = "h2" }
  auto_ipv4_hint  = true
  depends_on      = [technitium_record_a.first]
}

resource "technitium_record_a" "first" {
  zone       = technitium_zone.test.zone
  domain     = "www.hint.test"
  ip_address = "192.0.2.1"
}
