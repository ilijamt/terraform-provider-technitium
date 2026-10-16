variable "client_id" {
  type = string
}

variable "records" {
  type = number
}

resource "technitium_settings_sso" "this" {
  enabled   = true
  authority = "https://idp.example.com/realms/home"
  client_id = var.client_id
}

resource "technitium_zone" "test" {
  count = 8
  zone  = "restart${count.index}.test"
}

resource "technitium_record_a" "test" {
  count      = var.records
  zone       = technitium_zone.test[count.index % 8].zone
  domain     = "r${count.index}.${technitium_zone.test[count.index % 8].zone}"
  ip_address = "10.0.0.1"
  depends_on = [technitium_settings_sso.this]
}
