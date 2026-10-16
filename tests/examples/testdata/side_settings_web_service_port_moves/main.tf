variable "port" {
  type = number
}

resource "technitium_settings_web_service" "this" {
  web_service_http_port = var.port
}

resource "technitium_zone" "after" {
  zone       = "moved.example"
  type       = "Primary"
  depends_on = [technitium_settings_web_service.this]
}
