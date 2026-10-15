variable "tsig_secret" {
  type      = string
  sensitive = true
}

resource "technitium_settings_tsig" "this" {
  tsig_keys = [{
    key_name       = "transfer"
    shared_secret  = var.tsig_secret
    algorithm_name = "hmac-sha256"
  }]
}
