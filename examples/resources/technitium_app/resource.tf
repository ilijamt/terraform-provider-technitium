resource "technitium_app" "split_horizon" {
  name = "Split Horizon"
  url  = "https://download.technitium.com/dns/apps/SplitHorizonApp-v11.zip"
}

# url is optional: leave it out to adopt an app that is already installed, and
# set it later to install or update the app from that URL.
resource "technitium_app" "adopted" {
  name = "Advanced Blocking"
}
