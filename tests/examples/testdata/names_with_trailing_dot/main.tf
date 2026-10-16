resource "technitium_zone" "test" {
  zone = "dot.test"
}

resource "technitium_record_cname" "www" {
  zone   = technitium_zone.test.zone
  domain = "www.dot.test"
  cname  = "Target.Example.NET."
}

resource "technitium_record_dname" "old" {
  zone   = technitium_zone.test.zone
  domain = "old.dot.test"
  dname  = "new.example.net."
}

resource "technitium_record_soa" "soa" {
  zone                = technitium_zone.test.zone
  domain              = technitium_zone.test.zone
  primary_name_server = "ns1.dot.test."
  responsible_person  = "hostmaster.dot.test."
}
