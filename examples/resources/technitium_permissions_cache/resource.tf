resource "technitium_group" "perms_cache_ops" {
  name = "Ops cache"
}

# Groups the server keeps with full rights need not be listed.
resource "technitium_permissions_cache" "this" {
  group_permissions = [
    { name = technitium_group.perms_cache_ops.name, can_view = true, can_modify = false, can_delete = false },
  ]
}
