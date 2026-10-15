resource "technitium_api_token" "external_dns" {
  username   = technitium_user.external_dns.username
  token_name = "external-dns"
}

output "external_dns_token" {
  value     = technitium_api_token.external_dns.token
  sensitive = true
}
