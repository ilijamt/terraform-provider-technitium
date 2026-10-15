resource "technitium_zone" "svcb_home" {
  zone = "svcb.example.com"
}

resource "technitium_record_svcb" "web" {
  zone            = technitium_zone.svcb_home.zone
  domain          = technitium_zone.svcb_home.zone
  svc_priority    = 1
  svc_target_name = "."
  svc_params = {
    alpn = "h2,h3"
    port = "443"
  }
}
