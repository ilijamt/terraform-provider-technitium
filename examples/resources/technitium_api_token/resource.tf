variable "external_dns_password" {
  type      = string
  sensitive = true
}

resource "technitium_user" "token_external_dns" {
  username = "external-dns"
  password = var.external_dns_password
}

resource "technitium_api_token" "external_dns" {
  username   = technitium_user.token_external_dns.username
  token_name = "external-dns"
}

output "external_dns_token" {
  value     = technitium_api_token.external_dns.token
  sensitive = true
}
