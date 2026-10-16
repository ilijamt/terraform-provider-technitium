data "technitium_store_app" "split_horizon" {
  name = "Split Horizon"
}

resource "technitium_app" "split_horizon" {
  name = data.technitium_store_app.split_horizon.name
  url  = data.technitium_store_app.split_horizon.url
}

resource "technitium_app_config" "split_horizon" {
  name   = technitium_app.split_horizon.name
  config = jsonencode({ networks = { lan = ["10.0.0.0/8"] } })
}

resource "technitium_zone" "test" {
  zone = "apps.test"
}

resource "technitium_record_app" "www" {
  zone        = technitium_zone.test.zone
  domain      = "www.apps.test"
  app_name    = technitium_app.split_horizon.name
  class_path  = "SplitHorizon.SimpleAddress"
  record_data = jsonencode({ lan = ["10.0.0.1"] })
}
