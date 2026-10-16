resource "technitium_zone" "test" {
  zone = "app.test"
}

resource "technitium_record_app" "www" {
  zone        = technitium_zone.test.zone
  domain      = "www.app.test"
  app_name    = "Split Horizon"
  class_path  = "SplitHorizon.SimpleAddress"
  record_data = jsonencode({ public = ["192.0.2.1"], private = ["10.0.0.1"] })
}
