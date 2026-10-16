resource "technitium_zone" "cat" {
  zone = "conv-cat.test"
  type = "Catalog"
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
  type    = "Primary"
  catalog = technitium_zone.cat.zone
}

resource "technitium_zone" "fwd" {
  zone    = "conv-fwd.test"
  type    = "Forwarder"
  catalog = technitium_zone.cat.zone
}
