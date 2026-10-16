data "technitium_group" "dns_admins" {
  name = "DNS Administrators"
}

resource "technitium_settings_sso" "this" {
  enabled       = false
  authority     = "https://idp.example.com/realms/home"
  client_id     = "technitium"
  client_secret = "s3cret"
  allow_signup  = true
  group_map = [
    { remote_group = "dns-operators", local_group = data.technitium_group.dns_admins.name },
  ]
}

data "technitium_settings_sso" "this" {
  depends_on = [technitium_settings_sso.this]
}
