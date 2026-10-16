data "technitium_group" "dns_admins" {
  name = "DNS Administrators"
}

# SSO stays disabled: enabling it would make the server restart its web
# service against a provider that does not exist.
resource "technitium_settings_sso" "this" {
  enabled       = false
  authority     = "https://idp.example.com/realms/home"
  client_id     = "technitium"
  client_secret = "s3cret"
  allow_signup  = true
  group_map = [
    { remote_group = "dns-admins", local_group = data.technitium_group.dns_admins.name },
  ]
}

data "technitium_settings_sso" "this" {
  depends_on = [technitium_settings_sso.this]
}
