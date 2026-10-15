data "technitium_settings_recursion" "this" {}

output "recursion" {
  value = data.technitium_settings_recursion.this.recursion
}
