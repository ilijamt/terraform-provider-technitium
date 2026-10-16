variable "port" {
  type = number
}

resource "technitium_settings_web_service" "this" {
  web_service_http_port = var.port
}
