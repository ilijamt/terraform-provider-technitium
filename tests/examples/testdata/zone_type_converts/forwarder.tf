resource "technitium_zone" "test" {
  zone         = "convert2.test"
  type         = "Forwarder"
  query_access = "AllowOnlyPrivateNetworks"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.convert2.test"
  ip_address = "10.0.0.1"
}
