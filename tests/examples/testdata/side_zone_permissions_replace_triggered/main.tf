resource "technitium_zone" "z" {
  zone = "side-replace.test"
  type = "Primary"
}

resource "technitium_user" "u" {
  username = "sidereplace"
  password = "sidereplace-pass"
}

resource "technitium_zone_permissions" "z" {
  zone = technitium_zone.z.zone
  user_permissions = [
    { username = technitium_user.u.username, can_view = true, can_modify = true, can_delete = false },
  ]

  lifecycle {
    replace_triggered_by = [technitium_zone.z]
  }
}
