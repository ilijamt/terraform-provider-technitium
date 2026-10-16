resource "technitium_user" "alice" {
  username     = "alice"
  password     = "second-secret"
  display_name = "Alice"
}

resource "technitium_group" "ops" {
  name        = "Ops"
  description = "Operations"
}

resource "technitium_group_members" "ops" {
  group   = technitium_group.ops.name
  members = [technitium_user.alice.username]
}

resource "technitium_permissions_zones" "this" {
  user_permissions = [
    { username = technitium_user.alice.username, can_view = true, can_modify = false, can_delete = false },
  ]
  group_permissions = [
    { name = technitium_group.ops.name, can_view = true, can_modify = true, can_delete = false },
  ]
}
