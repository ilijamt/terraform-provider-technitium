resource "technitium_zone" "test" {
  zone = "perm.test"
}

resource "technitium_user" "bob" {
  username = "bob"
  password = "bob-secret"
}

resource "technitium_group" "ops" {
  name = "ZoneOps"
}

resource "technitium_zone_permissions" "test" {
  zone = technitium_zone.test.zone
  user_permissions = [
    { username = technitium_user.bob.username, can_view = true, can_modify = true, can_delete = false },
  ]
  group_permissions = [
    { name = technitium_group.ops.name, can_view = true, can_modify = false, can_delete = false },
  ]
}

data "technitium_zone_permissions" "test" {
  zone       = technitium_zone_permissions.test.zone
  depends_on = [technitium_zone_permissions.test]
}
