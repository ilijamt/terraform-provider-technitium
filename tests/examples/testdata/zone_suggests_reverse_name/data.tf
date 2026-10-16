variable "zone" {
  type = string
}

data "technitium_zone" "test" {
  zone = var.zone
}
