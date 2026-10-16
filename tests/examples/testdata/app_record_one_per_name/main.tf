resource "technitium_zone" "test" {
  zone = "appone.test"
}

resource "technitium_record_app" "a" {
  zone        = technitium_zone.test.zone
  domain      = "www.appone.test"
  app_name    = "Missing App"
  class_path  = "Missing.A"
  record_data = jsonencode({})
}

resource "technitium_record_app" "b" {
  zone        = technitium_zone.test.zone
  domain      = "www.appone.test"
  app_name    = "Missing App"
  class_path  = "Missing.B"
  record_data = jsonencode({})
  depends_on  = [technitium_record_app.a]
}
