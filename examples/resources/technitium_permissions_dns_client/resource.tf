resource "technitium_group" "perms_dns_client_ops" {
  name = "Ops dns_client"
}

# Groups the server keeps with full rights need not be listed.
resource "technitium_permissions_dns_client" "this" {
  group_permissions = [
    { name = technitium_group.perms_dns_client_ops.name, can_view = true, can_modify = false, can_delete = false },
  ]
}
