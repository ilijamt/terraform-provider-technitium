resource "technitium_zone" "catalog" {
  zone = "catalog.test"
  type = "Catalog"
}

resource "technitium_zone" "created" {
  zone       = "created.test"
  catalog    = "Catalog.TEST."
  depends_on = [technitium_zone.catalog]
}
