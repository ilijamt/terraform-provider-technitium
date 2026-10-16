resource "technitium_zone" "test" {
  zone = "case.test"
}

resource "technitium_record_txt" "hello" {
  zone   = technitium_zone.test.zone
  domain = "case.test"
  text   = "hello"
}

resource "technitium_record_caa" "issue" {
  zone   = technitium_zone.test.zone
  domain = "case.test"
  flags  = 0
  tag    = "issue"
  value  = "ca.example"
}
