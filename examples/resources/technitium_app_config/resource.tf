resource "technitium_app" "config_split_horizon" {
  name = "Split Horizon"
  url  = "https://download.technitium.com/dns/apps/SplitHorizonApp-v11.zip"
}

resource "technitium_app_config" "split_horizon" {
  name = technitium_app.config_split_horizon.name
  config = jsonencode({
    networks = {
      lan = ["192.168.0.0/16"]
    }
  })
}
