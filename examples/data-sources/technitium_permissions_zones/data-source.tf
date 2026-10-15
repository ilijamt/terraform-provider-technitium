# The server always keeps the built-in admin groups in a section, so take
# their rows from the server and add only your own.
data "technitium_permissions_zones" "this" {}

resource "technitium_permissions_zones" "this" {
  group_permissions = setunion(
    [for p in data.technitium_permissions_zones.this.group_permissions : p if contains(["Administrators", "DNS Administrators"], p.name)],
    [{ name = technitium_group.ops.name, can_view = true, can_modify = true, can_delete = false }],
  )
}
