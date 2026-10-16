resource "technitium_permissions_logs" "p" {
  group_permissions = [
    { name = "No Such Group", can_view = true, can_modify = false, can_delete = false },
  ]
}
