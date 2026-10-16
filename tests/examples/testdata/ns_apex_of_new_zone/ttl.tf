variable "server" {
  type = string
}

resource "technitium_zone" "test" {
  zone = "nsapex.test"
}

resource "technitium_record_ns" "apex" {
  zone        = technitium_zone.test.zone
  domain      = "nsapex.test"
  name_server = var.server
  ttl         = 300
}
