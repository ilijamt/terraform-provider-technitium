data "technitium_group" "administrators" {
  name = "Administrators"
}

resource "technitium_group_members" "administrators" {
  group   = data.technitium_group.administrators.name
  members = ["admin"]
}
