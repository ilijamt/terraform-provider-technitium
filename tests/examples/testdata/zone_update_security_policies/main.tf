resource "technitium_zone" "test" {
  zone   = "policy.test"
  update = "Allow"
  update_security_policies = [
    { tsig_key_name = "update-key", domain = "policy.test", allowed_types = ["A", "AAAA"] },
    { tsig_key_name = "update-key", domain = "*.policy.test", allowed_types = ["TXT"] },
  ]
}
