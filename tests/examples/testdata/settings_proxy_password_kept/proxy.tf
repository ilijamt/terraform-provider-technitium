variable "forwarder" {
  type = string
}

variable "password" {
  type = string
}

resource "technitium_settings_proxy_forwarders" "this" {
  proxy_type     = "Socks5"
  proxy_address  = "127.0.0.1"
  proxy_port     = 3128
  proxy_username = "proxyuser"
  proxy_password = var.password
  forwarders     = [var.forwarder]
}
