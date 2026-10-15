resource "technitium_group" "perms_allowed_ops" {
  name = "Ops allowed"
}

# Groups the server keeps with full rights need not be listed.
resource "technitium_permissions_allowed" "this" {
  group_permissions = [
    { name = technitium_group.perms_allowed_ops.name, can_view = true, can_modify = false, can_delete = false },
  ]
}
