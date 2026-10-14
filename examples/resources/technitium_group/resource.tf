resource "technitium_group" "ops" {
  name        = "Ops"
  description = "Operations"
  members     = [technitium_user.alice.username]
}
