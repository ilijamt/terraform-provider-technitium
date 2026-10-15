resource "technitium_group" "perms_dashboard_ops" {
  name = "Ops dashboard"
}

# Groups the server keeps with full rights need not be listed.
resource "technitium_permissions_dashboard" "this" {
  group_permissions = [
    { name = technitium_group.perms_dashboard_ops.name, can_view = true, can_modify = false, can_delete = false },
  ]
}
