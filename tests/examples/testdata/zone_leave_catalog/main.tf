resource "technitium_zone" "catalog" {
  zone = "leave-catalog.test"
  type = "Catalog"
}

resource "technitium_zone" "member" {
  zone       = "leave-member.test"
  depends_on = [technitium_zone.catalog]
}
