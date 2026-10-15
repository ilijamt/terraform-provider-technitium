resource "technitium_settings_blocking" "this" {
  enable_blocking                  = true
  blocking_type                    = "NxDomain"
  block_list_urls                  = ["https://big.oisd.nl/domainswild"]
  block_list_update_interval_hours = 24
}
