resource "technitium_zone" "dname_home" {
  zone = "dname.example.com"
}

resource "technitium_record_dname" "legacy" {
  zone   = technitium_zone.dname_home.zone
  domain = "old.dname.example.com"
  dname  = "new.example.net"
}
