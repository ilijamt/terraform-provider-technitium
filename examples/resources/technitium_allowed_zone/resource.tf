# Allows a domain that a block list in technitium_settings_blocking blocks.
resource "technitium_allowed_zone" "cdn" {
  domain = "cdn.example.com"
}
