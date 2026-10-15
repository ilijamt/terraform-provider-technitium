# The zone name must be a domain name, not an IP address or CIDR; use the
# in-addr.arpa form for a reverse zone.
resource "technitium_zone" "home" {
  zone = "home.example.com"
}

# A Forwarder zone starts empty; its forwarders are FWD records at the apex.
resource "technitium_zone" "reverse" {
  zone = "178.168.192.in-addr.arpa"
  type = "Forwarder"
}

resource "technitium_record_fwd" "reverse" {
  zone      = technitium_zone.reverse.zone
  domain    = technitium_zone.reverse.zone
  forwarder = "192.168.178.2"
}
