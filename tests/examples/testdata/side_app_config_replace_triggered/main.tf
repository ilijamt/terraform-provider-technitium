variable "url" {
  type = string
}

resource "technitium_app" "a" {
  name = "Split Horizon"
  url  = var.url
}

resource "technitium_app_config" "a" {
  name   = technitium_app.a.name
  config = jsonencode({ networks = { side = ["10.9.8.0/24"] } })

  lifecycle {
    replace_triggered_by = [technitium_app.a]
  }
}
