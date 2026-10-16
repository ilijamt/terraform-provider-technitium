data "technitium_group" "dns_admins" {
  name = "DNS Administrators"
}

resource "technitium_settings_ldap" "this" {
  enabled            = false
  server             = "ldap.example.com"
  port               = 636
  ssl_option         = "LDAPS"
  bind_username      = "CN=svc,DC=example,DC=com"
  bind_password      = "s3cret"
  search_base        = "DC=example,DC=com"
  user_search_filter = "(sAMAccountName={0})"
  group_attribute    = "memberOf"
  group_map = [
    { remote_group = "CN=dns-ops,DC=example,DC=com", local_group = data.technitium_group.dns_admins.name },
  ]
}

data "technitium_settings_ldap" "this" {
  depends_on = [technitium_settings_ldap.this]
}
