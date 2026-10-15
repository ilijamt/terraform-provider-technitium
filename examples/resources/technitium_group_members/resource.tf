variable "alice_password" {
  type      = string
  sensitive = true
}

resource "technitium_user" "members_alice" {
  username = "alice"
  password = var.alice_password
}

resource "technitium_group" "members_ops" {
  name = "Ops"
}

resource "technitium_group_members" "ops" {
  group   = technitium_group.members_ops.name
  members = [technitium_user.members_alice.username]

  # Deleting or replacing the group or a member makes the server drop the
  # membership. This sets the members again in the same apply.
  lifecycle {
    replace_triggered_by = [technitium_group.members_ops, technitium_user.members_alice]
  }
}

resource "technitium_group_members" "administrators" {
  group   = "Administrators"
  members = ["admin"]
}
