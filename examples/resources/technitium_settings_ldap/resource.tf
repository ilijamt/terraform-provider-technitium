data "technitium_group" "dns_administrators" {
  name = "DNS Administrators"
}

resource "technitium_settings_ldap" "this" {
  enabled            = true
  server             = "dc01.example.com"
  port               = 636
  ssl_option         = "LDAPS"
  bind_username      = "CN=svcDNS,OU=ServiceAccounts,DC=example,DC=com"
  bind_password      = var.ldap_bind_password
  search_base        = "DC=example,DC=com"
  user_search_filter = "(sAMAccountName={0})"
  group_attribute    = "memberOf"
  allow_signup       = true

  group_map = [
    { remote_group = "CN=DNS Admins,OU=Groups,DC=example,DC=com", local_group = data.technitium_group.dns_administrators.name },
  ]
}
