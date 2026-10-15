resource "technitium_zone" "naptr_home" {
  zone = "naptr.example.com"
}

resource "technitium_record_naptr" "sip" {
  zone              = technitium_zone.naptr_home.zone
  domain            = technitium_zone.naptr_home.zone
  naptr_order       = 100
  naptr_preference  = 10
  naptr_flags       = "S"
  naptr_services    = "SIP+D2U"
  naptr_replacement = "_sip._udp.naptr.example.com"
}
