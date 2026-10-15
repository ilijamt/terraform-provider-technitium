variable "alice_password" {
  type      = string
  sensitive = true
}

resource "technitium_zone" "perm_home" {
  zone = "home.example.com"
}

resource "technitium_user" "zone_perm_alice" {
  username = "alice"
  password = var.alice_password
}

resource "technitium_zone_permissions" "home" {
  zone = technitium_zone.perm_home.zone

  user_permissions = [
    { username = technitium_user.zone_perm_alice.username, can_view = true, can_modify = true, can_delete = false },
  ]

  # Administrators and DNS Administrators always keep full rights on a zone,
  # so only other groups are listed.
  group_permissions = []

  # Deleting or replacing the zone or user makes the server delete these
  # rights. This sets them again in the same apply.
  lifecycle {
    replace_triggered_by = [technitium_zone.perm_home, technitium_user.zone_perm_alice]
  }
}
