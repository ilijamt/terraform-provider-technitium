variable "ip" {
  type = string
}

resource "technitium_settings_web_service" "this" {
  web_service_local_addresses = [var.ip]
}

resource "technitium_zone" "after" {
  zone       = "still.example"
  type       = "Primary"
  depends_on = [technitium_settings_web_service.this]
}
