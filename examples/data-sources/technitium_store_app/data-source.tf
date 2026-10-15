data "technitium_store_app" "split_horizon" {
  name = "Split Horizon"
}

resource "technitium_app" "split_horizon" {
  name = data.technitium_store_app.split_horizon.name
  url  = data.technitium_store_app.split_horizon.url
}
