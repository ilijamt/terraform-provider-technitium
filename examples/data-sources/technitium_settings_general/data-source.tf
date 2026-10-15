data "technitium_settings_general" "this" {}

output "dns_server_domain" {
  value = data.technitium_settings_general.this.dns_server_domain
}
