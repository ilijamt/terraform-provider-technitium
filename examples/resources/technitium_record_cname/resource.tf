resource "technitium_zone" "cname_home" {
  zone = "cname.example.com"
}

resource "technitium_record_cname" "www" {
  zone   = technitium_zone.cname_home.zone
  domain = "www.cname.example.com"
  cname  = "web.cname.example.com"
  ttl    = 300
}
