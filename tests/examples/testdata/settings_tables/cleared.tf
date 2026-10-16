resource "technitium_settings_tsig" "this" {
  tsig_keys = []
}

resource "technitium_settings_general" "this" {
  qpm_prefix_limits_ipv4 = [{ prefix = 24, udp_limit = 6000, tcp_limit = 0 }]
}
