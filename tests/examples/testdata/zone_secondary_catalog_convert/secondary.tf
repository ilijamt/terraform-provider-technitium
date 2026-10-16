variable "ip" {
  type = string
}

resource "technitium_zone" "cat" {
  zone                          = "conv-cat.test"
  type                          = "SecondaryCatalog"
  primary_name_server_addresses = [var.ip]
}

import {
  to = technitium_zone.pri
  id = "conv-pri.test"
}

import {
  to = technitium_zone.fwd
  id = "conv-fwd.test"
}

resource "technitium_zone" "pri" {
  zone    = "conv-pri.test"
  type    = "Secondary"
  catalog = technitium_zone.cat.zone
}

resource "technitium_zone" "fwd" {
  zone    = "conv-fwd.test"
  type    = "SecondaryForwarder"
  catalog = technitium_zone.cat.zone
}
