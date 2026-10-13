resource "technitium_settings_proxy_forwarders" "this" {
  forwarders         = ["1.1.1.1", "9.9.9.9"]
  forwarder_protocol = "Https"
}
