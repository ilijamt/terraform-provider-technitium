variable "forwarder" {
  type = string
}

resource "technitium_settings_proxy_forwarders" "this" {
  forwarders = [var.forwarder]
}
