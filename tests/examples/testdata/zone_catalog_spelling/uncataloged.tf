resource "technitium_zone" "catalog" {
  zone = "catalog.test"
  type = "Catalog"
}

resource "technitium_zone" "updated" {
  zone       = "updated.test"
  catalog    = null
  depends_on = [technitium_zone.catalog]
}
