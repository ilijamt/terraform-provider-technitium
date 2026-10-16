variable "zone" {
  type = string
}

resource "technitium_zone" "test" {
  zone = var.zone
}
