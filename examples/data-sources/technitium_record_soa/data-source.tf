data "technitium_record_soa" "home" {
  zone   = "home.example.com"
  domain = "home.example.com"
}

output "home_serial" {
  value = data.technitium_record_soa.home.serial
}
