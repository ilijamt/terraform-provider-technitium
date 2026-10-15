# Port, address and TLS changes restart the web service, so apply them on
# their own and change the provider host when the port changes.
resource "technitium_settings_web_service" "this" {
  web_service_http_port                       = 5380
  web_service_enable_tls                      = true
  web_service_use_self_signed_tls_certificate = true
  web_service_tls_port                        = 53443
}
