variable "ip" {
  type = string
}

resource "technitium_zone" "cat" {
  zone                          = "conv-cat.test"
  type                          = "SecondaryCatalog"
  primary_name_server_addresses = [var.ip]
}
