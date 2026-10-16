resource "technitium_zone" "test" {
  zone = "naptr.test"
}

resource "technitium_record_naptr" "sip" {
  zone             = technitium_zone.test.zone
  domain           = "naptr.test"
  ttl              = 300
  naptr_order      = 100
  naptr_preference = 10
  naptr_flags      = "U"
  naptr_services   = "E2U+sip"
  naptr_regexp     = "!^.*$!sip:info@naptr.test!"
}

resource "technitium_record_rp" "admin" {
  zone       = technitium_zone.test.zone
  domain     = "naptr.test"
  mailbox    = "admin@naptr.test"
  txt_domain = "info.naptr.test"
}
