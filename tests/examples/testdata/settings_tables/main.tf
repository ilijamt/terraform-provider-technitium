resource "technitium_settings_tsig" "this" {
  tsig_keys = [
    { key_name = "zeta", shared_secret = "emV0YS1zZWNyZXQ=", algorithm_name = "hmac-sha256" },
    { key_name = "alpha", shared_secret = "YWxwaGEtc2VjcmV0", algorithm_name = "hmac-sha512" },
  ]
}

resource "technitium_settings_general" "this" {
  qpm_prefix_limits_ipv4 = [{ prefix = 24, udp_limit = 6000, tcp_limit = 0 }]
}
