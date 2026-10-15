resource "technitium_group" "perms_settings_ops" {
  name = "Ops settings"
}

# Groups the server keeps with full rights need not be listed.
resource "technitium_permissions_settings" "this" {
  group_permissions = [
    { name = technitium_group.perms_settings_ops.name, can_view = true, can_modify = false, can_delete = false },
  ]
}
