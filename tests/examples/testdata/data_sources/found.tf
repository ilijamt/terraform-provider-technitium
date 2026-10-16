resource "technitium_zone" "test" {
  zone = "data.test"
}

data "technitium_zone" "test" {
  zone = technitium_zone.test.zone
}

data "technitium_group" "admins" {
  name = "Administrators"
}

data "technitium_user" "admin" {
  username = "admin"
}

data "technitium_settings_general" "this" {}

resource "technitium_dhcp_scope" "test" {
  name             = "data"
  starting_address = "192.168.80.10"
  ending_address   = "192.168.80.20"
  subnet_mask      = "255.255.255.0"
}

data "technitium_dhcp_scope" "test" {
  name = technitium_dhcp_scope.test.name
}

resource "technitium_group" "ops" {
  name = "DataOps"
}

# The built-in groups the server keeps need not be listed.
resource "technitium_permissions_zones" "this" {
  group_permissions = [{ name = technitium_group.ops.name, can_view = true, can_modify = false, can_delete = false }]
}

data "technitium_permissions_zones" "this" {
  depends_on = [technitium_permissions_zones.this]
}
