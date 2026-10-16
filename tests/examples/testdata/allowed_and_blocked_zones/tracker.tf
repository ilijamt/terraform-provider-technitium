resource "technitium_blocked_zone" "ads" {
  domain = "tracker.block.test"
}

resource "technitium_allowed_zone" "cdn" {
  domain = "cdn.ads.block.test"
}
