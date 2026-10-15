resource "technitium_zone" "app_home" {
  zone = "app.example.com"
}

resource "technitium_record_app" "dyn" {
  zone       = technitium_zone.app_home.zone
  domain     = "dyn.app.example.com"
  app_name   = "Split Horizon"
  class_path = "SplitHorizon.SimpleAddress"
  record_data = jsonencode({
    "192.168.0.0/16" = ["192.168.1.10"]
    "0.0.0.0/0"      = ["203.0.113.10"]
  })
}
