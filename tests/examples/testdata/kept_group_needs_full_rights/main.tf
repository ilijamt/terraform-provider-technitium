resource "technitium_permissions_dashboard" "this" {
  group_permissions = [{ name = "Administrators", can_view = true, can_modify = false, can_delete = false }]
}
