resource "technitium_zone" "z" {
  zone = "side-trigger.test"
}

resource "technitium_user" "u" {
  username = "sidetrigger"
  password = "sidetrigger-pass"
}

resource "technitium_group" "g" {
  name = "SideTrigger"
}

resource "technitium_group_members" "g" {
  group   = technitium_group.g.name
  members = [technitium_user.u.username]

  lifecycle {
    replace_triggered_by = [technitium_user.u, technitium_group.g]
  }
}

resource "technitium_zone_permissions" "z" {
  zone = technitium_zone.z.zone
  user_permissions = [
    { username = technitium_user.u.username, can_view = true, can_modify = false, can_delete = false },
  ]
  group_permissions = [
    { name = technitium_group.g.name, can_view = true, can_modify = false, can_delete = false },
  ]

  lifecycle {
    replace_triggered_by = [technitium_zone.z, technitium_user.u, technitium_group.g]
  }
}

resource "technitium_api_token" "t" {
  username   = technitium_user.u.username
  token_name = "side"

  lifecycle {
    replace_triggered_by = [technitium_user.u]
  }
}
