resource "technitium_group" "perms_apps_ops" {
  name = "Ops apps"
}

# Groups the server keeps with full rights need not be listed.
resource "technitium_permissions_apps" "this" {
  group_permissions = [
    { name = technitium_group.perms_apps_ops.name, can_view = true, can_modify = false, can_delete = false },
  ]

  # Deleting or replacing the group makes the server delete its rights.
  # This sets them again in the same apply.
  lifecycle {
    replace_triggered_by = [technitium_group.perms_apps_ops]
  }
}
