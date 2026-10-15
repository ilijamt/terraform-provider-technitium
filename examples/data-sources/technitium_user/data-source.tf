data "technitium_user" "admin" {
  username = "admin"
}

output "admin_is_sso" {
  value = data.technitium_user.admin.is_sso_user
}
