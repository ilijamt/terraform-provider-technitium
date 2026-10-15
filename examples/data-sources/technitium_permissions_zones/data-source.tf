data "technitium_permissions_zones" "this" {}

output "zone_groups" {
  value = [for p in data.technitium_permissions_zones.this.group_permissions : p.name]
}
