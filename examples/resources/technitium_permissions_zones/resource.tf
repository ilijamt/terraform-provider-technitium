resource "technitium_permissions_zones" "this" {
  user_permissions = [
    { username = technitium_user.alice.username, can_view = true, can_modify = false, can_delete = false },
  ]

  # Administrators and DNS Administrators always keep full rights here, so
  # they need not be listed.
  group_permissions = [
    { name = technitium_group.ops.name, can_view = true, can_modify = true, can_delete = false },
  ]
}
