resource "technitium_settings_optional_protocols" "this" {
  enable_dns_over_http = true
  dns_over_http_port   = 8053
}
