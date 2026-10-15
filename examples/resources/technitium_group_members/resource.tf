resource "technitium_group_members" "ops" {
  group   = technitium_group.ops.name
  members = [technitium_user.alice.username]
}

# Built-in groups exist on every server; manage only their members.
resource "technitium_group_members" "administrators" {
  group   = "Administrators"
  members = ["admin"]
}
