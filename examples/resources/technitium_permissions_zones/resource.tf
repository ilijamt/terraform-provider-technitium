resource "technitium_permissions_zones" "this" {
  user_permissions = [
    { username = technitium_user.alice.username, can_view = true, can_modify = false, can_delete = false },
  ]

  group_permissions = [
    # The server always keeps the built-in admin groups, so list them as it has them.
    { name = "Administrators", can_view = true, can_modify = true, can_delete = true },
    { name = "DNS Administrators", can_view = true, can_modify = true, can_delete = true },
    { name = technitium_group.ops.name, can_view = true, can_modify = true, can_delete = false },
  ]
}
