resource "technitium_zone" "test" {
  zone = "self.test"
}

resource "technitium_record_https" "self" {
  zone            = technitium_zone.test.zone
  domain          = "self.test"
  svc_priority    = 1
  svc_target_name = "."
  svc_params      = {}
  ttl             = 600
}
