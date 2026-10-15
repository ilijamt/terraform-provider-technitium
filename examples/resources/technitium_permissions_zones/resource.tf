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
}
