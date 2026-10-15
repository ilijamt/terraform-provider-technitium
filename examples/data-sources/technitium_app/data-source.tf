data "technitium_app" "split_horizon" {
  name = "Split Horizon"
}

output "split_horizon_version" {
  value = data.technitium_app.split_horizon.version
}
