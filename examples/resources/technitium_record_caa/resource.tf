resource "technitium_zone" "caa_home" {
  zone = "caa.example.com"
}

resource "technitium_record_caa" "issue" {
  zone   = technitium_zone.caa_home.zone
  domain = technitium_zone.caa_home.zone
  flags  = 0
  tag    = "issue"
  value  = "letsencrypt.org"
}
