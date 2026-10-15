data "technitium_group_members" "administrators" {
  group = "Administrators"
}

output "administrators" {
  value = data.technitium_group_members.administrators.members
}
