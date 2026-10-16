resource "technitium_zone" "txt" {
  zone = "txt.test"
}

resource "technitium_record_txt" "spf" {
  zone   = technitium_zone.txt.zone
  domain = "txt.test"
  text   = "v=spf1 ip4:10.0.0.0/8 -all"
}
