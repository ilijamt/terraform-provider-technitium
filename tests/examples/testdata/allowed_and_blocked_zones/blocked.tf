resource "technitium_blocked_zone" "ads" {
  domain = "ads.block.test"
}

resource "technitium_allowed_zone" "cdn" {
  domain = "cdn.ads.block.test"
}
