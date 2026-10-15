resource "technitium_group" "perms_apps_ops" {
  name = "Ops apps"
}

# Groups the server keeps with full rights need not be listed.
resource "technitium_permissions_apps" "this" {
  group_permissions = [
    { name = technitium_group.perms_apps_ops.name, can_view = true, can_modify = false, can_delete = false },
  ]
}
