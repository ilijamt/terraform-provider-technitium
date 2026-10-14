resource "technitium_user" "alice" {
  username     = "alice"
  password     = var.alice_password
  display_name = "Alice"
}
