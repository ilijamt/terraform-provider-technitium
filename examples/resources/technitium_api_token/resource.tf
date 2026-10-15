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

  # Deleting or replacing the user makes the server delete its tokens. This
  # issues a new one in the same apply.
  lifecycle {
    replace_triggered_by = [technitium_user.token_external_dns]
  }
}

output "external_dns_token" {
  value     = technitium_api_token.external_dns.token
  sensitive = true
}
