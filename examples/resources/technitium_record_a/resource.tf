resource "technitium_record_a" "nas" {
  zone       = "home.example.com"
  domain     = "nas.home.example.com"
  ip_address = "192.168.178.10"
  ttl        = 300
  ptr        = true
}
