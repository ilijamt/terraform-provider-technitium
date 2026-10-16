resource "technitium_settings_general" "this" {
  dns_server_domain = "ns.settings.test"
}

resource "technitium_settings_proxy_forwarders" "this" {
  forwarders = ["1.1.1.1", "9.9.9.9"]
}

resource "technitium_settings_recursion" "this" {
  recursion = "Allow"
}
