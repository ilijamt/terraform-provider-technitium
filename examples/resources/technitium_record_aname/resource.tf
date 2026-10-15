resource "technitium_zone" "aname_home" {
  zone = "aname.example.com"
}

resource "technitium_record_aname" "apex" {
  zone   = technitium_zone.aname_home.zone
  domain = technitium_zone.aname_home.zone
  aname  = "lb.example.net"
}
