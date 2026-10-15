resource "technitium_group" "perms_logs_ops" {
  name = "Ops logs"
}

# Groups the server keeps with full rights need not be listed.
resource "technitium_permissions_logs" "this" {
  group_permissions = [
    { name = technitium_group.perms_logs_ops.name, can_view = true, can_modify = false, can_delete = false },
  ]
}
