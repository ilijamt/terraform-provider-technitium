variable "primary" {
  type = string
}

resource "technitium_zone" "secondary" {
  zone                           = "secondary.test"
  type                           = "Secondary"
  primary_name_server_addresses  = [var.primary]
  primary_zone_transfer_protocol = "Tcp"
}

resource "technitium_zone" "stub" {
  zone                          = "stub.test"
  type                          = "Stub"
  primary_name_server_addresses = [var.primary]
}

resource "technitium_zone" "secfwd" {
  zone                          = "secfwd.test"
  type                          = "SecondaryForwarder"
  primary_name_server_addresses = [var.primary]
}
