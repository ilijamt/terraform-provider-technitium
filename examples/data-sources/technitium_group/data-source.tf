# The built-in groups exist on every server: read them instead of managing them.
data "technitium_group" "administrators" {
  name = "Administrators"
}

resource "technitium_group_members" "administrators" {
  group   = data.technitium_group.administrators.name
  members = ["admin"]
}
