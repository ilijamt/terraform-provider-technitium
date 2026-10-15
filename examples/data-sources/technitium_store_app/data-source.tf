# Install the newest version from the DNS App Store. When the store has a newer
# version, the URL changes and the next apply updates the app in place.
data "technitium_store_app" "split_horizon" {
  name = "Split Horizon"
}

resource "technitium_app" "split_horizon" {
  name = data.technitium_store_app.split_horizon.name
  url  = data.technitium_store_app.split_horizon.url
}
