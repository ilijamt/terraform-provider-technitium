variable "primary" {
  type = string
}

# The server adds the port for Tls and Quic, so the config carries it.
resource "technitium_zone" "secondary" {
  zone                           = "secondary.test"
  type                           = "Secondary"
  primary_name_server_addresses  = ["${var.primary}:853"]
  primary_zone_transfer_protocol = "Tls"
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
