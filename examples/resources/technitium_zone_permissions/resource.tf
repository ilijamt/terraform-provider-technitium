resource "technitium_zone_permissions" "home" {
  zone = technitium_zone.home.zone

  user_permissions = [
    { username = technitium_user.alice.username, can_view = true, can_modify = true, can_delete = false },
  ]

  # Administrators and DNS Administrators always keep full rights on a zone,
  # so only other groups are listed.
  group_permissions = []
}
