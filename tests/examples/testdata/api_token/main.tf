resource "technitium_user" "ci" {
  username = "ci"
  password = "ci-secret"
}

resource "technitium_api_token" "ci" {
  username   = technitium_user.ci.username
  token_name = "deploy"
}
