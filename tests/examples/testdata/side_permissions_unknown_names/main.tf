resource "technitium_zone" "z" {
  zone = "side-unknown.test"
}

resource "technitium_zone_permissions" "z" {
  zone = technitium_zone.z.zone
  user_permissions = [
    { username = "nosuchuser", can_view = true, can_modify = false, can_delete = false },
  ]
}
