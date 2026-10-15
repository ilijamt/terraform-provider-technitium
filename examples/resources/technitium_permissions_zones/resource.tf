variable "alice_password" {
  type      = string
  sensitive = true
}

resource "technitium_user" "perms_alice" {
  username = "alice"
  password = var.alice_password
}

resource "technitium_group" "perms_ops" {
  name = "Ops"
}

resource "technitium_permissions_zones" "this" {
  user_permissions = [
    { username = technitium_user.perms_alice.username, can_view = true, can_modify = false, can_delete = false },
  ]

  # Administrators and DNS Administrators always keep full rights here, so
  # they need not be listed.
  group_permissions = [
    { name = technitium_group.perms_ops.name, can_view = true, can_modify = true, can_delete = false },
  ]

  # Deleting or replacing the user or group makes the server delete its rights.
  # This sets them again in the same apply.
  lifecycle {
    replace_triggered_by = [technitium_user.perms_alice, technitium_group.perms_ops]
  }
}
