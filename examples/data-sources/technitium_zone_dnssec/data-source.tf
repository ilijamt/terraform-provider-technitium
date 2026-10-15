data "technitium_zone_dnssec" "home" {
  zone = "home.example.com"
}

output "home_signed" {
  value = data.technitium_zone_dnssec.home.signed
}
