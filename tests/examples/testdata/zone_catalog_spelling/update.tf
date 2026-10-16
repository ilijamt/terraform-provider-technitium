resource "technitium_zone" "catalog" {
  zone = "catalog.test"
  type = "Catalog"
}

resource "technitium_zone" "updated" {
  zone       = "updated.test"
  catalog    = "CATALOG.test."
  depends_on = [technitium_zone.catalog]
}
