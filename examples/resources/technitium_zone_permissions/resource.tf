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
}
