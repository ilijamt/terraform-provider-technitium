resource "technitium_settings_cache" "this" {
  serve_stale           = true
  serve_stale_ttl       = 259200
  enable_cache_prefetch = true
}
