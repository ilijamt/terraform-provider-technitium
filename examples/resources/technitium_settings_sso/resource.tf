data "technitium_group" "dns_administrators" {
  name = "DNS Administrators"
}

resource "technitium_settings_sso" "this" {
  enabled                            = true
  authority                          = "https://auth.example.com/realms/home"
  client_id                          = "technitium"
  client_secret                      = var.sso_client_secret
  allow_signup                       = true
  allow_signup_only_for_mapped_users = true

  group_map = [
    { remote_group = "dns-admins", local_group = data.technitium_group.dns_administrators.name },
  ]
}
