resource "technitium_zone" "https_home" {
  zone = "https.example.com"
}

resource "technitium_record_https" "web" {
  zone            = technitium_zone.https_home.zone
  domain          = technitium_zone.https_home.zone
  svc_priority    = 1
  svc_target_name = "."
  svc_params = {
    alpn = "h2,h3"
    port = "443"
  }
}
