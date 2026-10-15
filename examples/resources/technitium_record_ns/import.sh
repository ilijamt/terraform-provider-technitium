# zone/domain/name_server
terraform import technitium_record_ns.delegated ns.example.com/sub.ns.example.com/ns1.sub.ns.example.com

# The apex NS record a new primary zone gets, named after the server domain.
terraform import technitium_record_ns.apex ns.example.com/ns.example.com/dns.example.net
