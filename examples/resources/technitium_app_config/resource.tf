resource "technitium_app_config" "split_horizon" {
  name = technitium_app.split_horizon.name
  config = jsonencode({
    networks = {
      lan = ["192.168.0.0/16"]
    }
  })
}
