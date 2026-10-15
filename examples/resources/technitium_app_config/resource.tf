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

  # Uninstalling or replacing the app makes the server delete its config.
  # This sets it again in the same apply.
  lifecycle {
    replace_triggered_by = [technitium_app.config_split_horizon]
  }
}
