resource "technitium_settings_web_service" "this" {
  web_service_http_port  = 5380
  web_service_enable_tls = true
  web_service_tls_port   = 53443
}
