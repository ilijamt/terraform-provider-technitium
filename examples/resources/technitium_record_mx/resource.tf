resource "technitium_zone" "mx_home" {
  zone = "mx.example.com"
}

resource "technitium_record_mx" "mail" {
  zone       = technitium_zone.mx_home.zone
  domain     = technitium_zone.mx_home.zone
  exchange   = "mail.mx.example.com"
  preference = 10
}
