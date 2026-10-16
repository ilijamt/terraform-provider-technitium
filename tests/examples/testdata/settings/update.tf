resource "technitium_settings_general" "this" {
  dns_server_domain = "ns.settings.test"
}

resource "technitium_settings_proxy_forwarders" "this" {
  forwarders = []
}

resource "technitium_settings_recursion" "this" {
  recursion = "Allow"
}
