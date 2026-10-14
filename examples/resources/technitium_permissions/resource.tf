resource "technitium_permissions" "zones" {
  section = "Zones"

  group_permissions = [
    # The server always keeps the built-in admin groups, so list them as it has them.
    { name = "Administrators", can_view = true, can_modify = true, can_delete = true },
    { name = "DNS Administrators", can_view = true, can_modify = true, can_delete = true },
    { name = technitium_group.ops.name, can_view = true, can_modify = true, can_delete = false },
  ]
}
