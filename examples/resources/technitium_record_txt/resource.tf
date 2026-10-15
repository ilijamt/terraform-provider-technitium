resource "technitium_zone" "txt_home" {
  zone = "txt.example.com"
}

resource "technitium_record_txt" "hello" {
  zone   = technitium_zone.txt_home.zone
  domain = "hello.txt.example.com"
  text   = "hello"
  ttl    = 300
}

resource "technitium_record_txt" "spf" {
  zone   = technitium_zone.txt_home.zone
  domain = technitium_zone.txt_home.zone
  text   = "v=spf1 ip4:192.168.178.0/24 -all"
}
