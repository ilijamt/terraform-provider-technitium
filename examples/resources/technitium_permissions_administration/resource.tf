resource "technitium_group" "perms_administration_ops" {
  name = "Ops administration"
}

# Groups the server keeps with full rights need not be listed.
resource "technitium_permissions_administration" "this" {
  group_permissions = [
    { name = technitium_group.perms_administration_ops.name, can_view = true, can_modify = false, can_delete = false },
  ]
}
