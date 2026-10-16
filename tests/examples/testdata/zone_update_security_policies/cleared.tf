resource "technitium_zone" "test" {
  zone                     = "policy.test"
  update                   = "Allow"
  update_security_policies = []
}
