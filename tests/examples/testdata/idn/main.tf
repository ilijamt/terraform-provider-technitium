resource "technitium_zone" "test" {
  zone = "bücher.test"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.bücher.test"
  ip_address = "10.0.0.1"
}

resource "technitium_record_cname" "shop" {
  zone   = technitium_zone.test.zone
  domain = "shop.bücher.test"
  cname  = "www.bücher.test"
}
