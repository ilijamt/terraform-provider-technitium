resource "technitium_settings_logging" "this" {
  logging_type      = "FileAndConsole"
  max_log_file_days = 30
}
