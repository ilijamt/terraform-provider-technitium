package technitium

import "github.com/ilijamt/terraform-provider-technitium/internal/framework"

const (
	// macAddress is how the server writes hardware addresses.
	macAddress = `^[0-9A-F]{2}(-[0-9A-F]{2}){5}$`
	// hexBytes is how the server writes option values.
	hexBytes = `^[0-9A-F]{2}(:[0-9A-F]{2})*$`
)

// dhcpScope is a DHCP scope. The set call creates it when it does not exist
// and changes only the parameters it gets. Its reserved leases are
// dhcpReservedLease resources.
var dhcpScope = framework.Spec{
	Name:        "dhcp_scope",
	Description: "Manages a DHCP scope. Reserved leases are `technitium_dhcp_reserved_lease` resources.",
	Read:        "/api/dhcp/scopes/get",
	Update:      "/api/dhcp/scopes/set",
	Delete:      "/api/dhcp/scopes/delete",
	Attrs: []framework.Attr{
		{Name: "name", Param: "name", Type: framework.String, Key: "name", Required: true, Identity: true, Update: true,
			Description: "The scope name. Changing it replaces the scope."},
		{Name: "starting_address", Param: "startingAddress", Type: framework.String, Key: "startingAddress", Required: true, Update: true,
			Description: "The first IP address the scope hands out."},
		{Name: "ending_address", Param: "endingAddress", Type: framework.String, Key: "endingAddress", Required: true, Update: true,
			Description: "The last IP address the scope hands out."},
		{Name: "subnet_mask", Param: "subnetMask", Type: framework.String, Key: "subnetMask", Required: true, Update: true,
			Description: "The subnet mask of the network, for example `255.255.255.0`."},
		{Name: "lease_time_days", Param: "leaseTimeDays", Type: framework.Int64, Key: "leaseTimeDays", Update: true,
			Description: "The days part of the lease time. Technitium's default is `7`."},
		{Name: "lease_time_hours", Param: "leaseTimeHours", Type: framework.Int64, Key: "leaseTimeHours", Update: true,
			Description: "The hours part of the lease time."},
		{Name: "lease_time_minutes", Param: "leaseTimeMinutes", Type: framework.Int64, Key: "leaseTimeMinutes", Update: true,
			Description: "The minutes part of the lease time."},
		{Name: "offer_delay_time", Param: "offerDelayTime", Type: framework.Int64, Key: "offerDelayTime", Update: true,
			Description: "The milliseconds to wait before sending an offer, so another DHCP server on the network can answer first."},
		{Name: "ping_check_enabled", Param: "pingCheckEnabled", Type: framework.Bool, Key: "pingCheckEnabled", Update: true,
			Description: "Ping an address before handing it out, to avoid conflicts with devices configured by hand."},
		{Name: "ping_check_timeout", Param: "pingCheckTimeout", Type: framework.Int64, Key: "pingCheckTimeout", Update: true,
			Description: "The milliseconds to wait for a ping reply."},
		{Name: "ping_check_retries", Param: "pingCheckRetries", Type: framework.Int64, Key: "pingCheckRetries", Update: true,
			Description: "The number of pings to try."},
		{Name: "domain_name", Param: "domainName", Type: framework.String, Key: "domainName", Update: true,
			Description: "The domain name of the network (option 15). With `dns_updates`, clients get forward and reverse DNS records in it."},
		{Name: "domain_search_list", Param: "domainSearchList", Type: framework.List, Key: "domainSearchList", Update: true,
			Description: "The domain names clients append when resolving short names (option 119)."},
		{Name: "dns_updates", Param: "dnsUpdates", Type: framework.Bool, Key: "dnsUpdates", Update: true,
			Description: "Add and remove forward and reverse DNS records for clients in `domain_name`."},
		{Name: "dns_overwrite_for_dynamic_lease", Param: "dnsOverwriteForDynamicLease", Type: framework.Bool, Key: "dnsOverwriteForDynamicLease", Update: true,
			Description: "Overwrite an existing `A` record with a client's name for dynamic leases."},
		{Name: "dns_ttl", Param: "dnsTtl", Type: framework.Int64, Key: "dnsTtl", Update: true,
			Description: "The TTL of the DNS records added for clients."},
		{Name: "server_address", Param: "serverAddress", Type: framework.String, Key: "serverAddress", Update: true,
			Description: "The address of the next (TFTP) server clients boot from (siaddr). Defaults to the DHCP server's own address."},
		{Name: "server_host_name", Param: "serverHostName", Type: framework.String, Key: "serverHostName", Update: true,
			Description: "The host name of the TFTP boot server (sname, option 66)."},
		{Name: "boot_file_name", Param: "bootFileName", Type: framework.String, Key: "bootFileName", Update: true,
			Description: "The boot file on the TFTP server (file, option 67)."},
		{Name: "router_address", Param: "routerAddress", Type: framework.String, Key: "routerAddress", Update: true,
			Description: "The default gateway clients use (option 3)."},
		{Name: "use_this_dns_server", Param: "useThisDnsServer", Type: framework.Bool, Key: "useThisDnsServer", Update: true,
			Description: "Give clients this DNS server's address as their DNS server, instead of `dns_servers`."},
		{Name: "dns_servers", Param: "dnsServers", Type: framework.List, Key: "dnsServers", Update: true,
			Description: "The DNS server addresses clients use (option 6). Ignored when `use_this_dns_server` is `true`."},
		{Name: "wins_servers", Param: "winsServers", Type: framework.List, Key: "winsServers", Update: true,
			Description: "The NetBIOS name server (WINS) addresses clients use (option 44)."},
		{Name: "ntp_servers", Param: "ntpServers", Type: framework.List, Key: "ntpServers", Update: true,
			Description: "The NTP server addresses clients use (option 42)."},
		{Name: "ntp_server_domain_names", Param: "ntpServerDomainNames", Type: framework.List, Key: "ntpServerDomainNames", Update: true,
			Description: "NTP server names, such as `pool.ntp.org`, that the server resolves and adds to the NTP servers clients get (option 42)."},
		{Name: "static_routes", Param: "staticRoutes", Type: framework.Table, Key: "staticRoutes", Update: true,
			Columns: []framework.Column{
				{Name: "destination", Key: "destination", Type: framework.String, Description: "The destination network address."},
				{Name: "subnet_mask", Key: "subnetMask", Type: framework.String, Description: "The destination subnet mask."},
				{Name: "router", Key: "router", Type: framework.String, Description: "The gateway for the destination."},
			},
			Description: "Routes clients use to reach other networks (option 121). An empty set removes them."},
		{Name: "vendor_info", Param: "vendorInfo", Type: framework.Table, Key: "vendorInfo", Update: true,
			Columns: []framework.Column{
				{Name: "identifier", Key: "identifier", Type: framework.String,
					Description: "The vendor class identifier, or an expression on it such as `substring(vendor-class-identifier,0,9)==\"PXEClient\"`."},
				{Name: "information", Key: "information", Type: framework.String, Pattern: hexBytes,
					Description: "The vendor specific information, as upper case hex bytes separated by colons, for example `06:01:03`."},
			},
			Description: "Vendor specific information (option 43) for clients of a vendor class. An empty set removes it."},
		{Name: "capwap_ac_ip_addresses", Param: "capwapAcIpAddresses", Type: framework.List, Key: "capwapAcIpAddresses", Update: true,
			Description: "The CAPWAP access controller addresses wireless access points connect to (option 138)."},
		{Name: "tftp_server_addresses", Param: "tftpServerAddresses", Type: framework.List, Key: "tftpServerAddresses", Update: true,
			Description: "The TFTP or VoIP configuration server addresses (option 150)."},
		{Name: "generic_options", Param: "genericOptions", Type: framework.Table, Key: "genericOptions", Update: true,
			Columns: []framework.Column{
				{Name: "code", Key: "code", Type: framework.Int64, Description: "The DHCP option code."},
				{Name: "value", Key: "value", Type: framework.String, Pattern: hexBytes,
					Description: "The option value, as upper case hex bytes separated by colons, for example `C0:A8:01:01`."},
			},
			Description: "DHCP options without an attribute of their own. An empty set removes them."},
		{Name: "exclusions", Param: "exclusions", Type: framework.Table, Key: "exclusions", Update: true,
			Columns: []framework.Column{
				{Name: "starting_address", Key: "startingAddress", Type: framework.String, Description: "The first excluded address."},
				{Name: "ending_address", Key: "endingAddress", Type: framework.String, Description: "The last excluded address."},
			},
			Description: "Address ranges in the scope that are never handed out dynamically. An empty set removes them."},
		{Name: "allow_only_reserved_leases", Param: "allowOnlyReservedLeases", Type: framework.Bool, Key: "allowOnlyReservedLeases", Update: true,
			Description: "Hand out reserved leases only, no dynamic addresses."},
		{Name: "block_locally_administered_mac_addresses", Param: "blockLocallyAdministeredMacAddresses", Type: framework.Bool, Key: "blockLocallyAdministeredMacAddresses", Update: true,
			Description: "Give no dynamic address to clients with a locally administered (randomized) MAC address."},
		{Name: "ignore_client_identifier_option", Param: "ignoreClientIdentifierOption", Type: framework.Bool, Key: "ignoreClientIdentifierOption", Update: true,
			Description: "Identify clients by MAC address only, ignoring the client identifier (option 61) they send. Changing it may give existing clients another address on renewal."},
	},
}

// dhcpReservedLease reserves an address in a scope for one device. There is
// no update call, so every change replaces it.
var dhcpReservedLease = framework.Spec{
	Name:        "dhcp_reserved_lease",
	Description: "Reserves an IP address in a DHCP scope for one device. Changing any attribute replaces the reservation.",
	Create:      "/api/dhcp/scopes/addReservedLease",
	Read:        "/api/dhcp/scopes/get",
	Delete:      "/api/dhcp/scopes/removeReservedLease",
	List:        "reservedLeases",
	Attrs: []framework.Attr{
		{Name: "scope", Param: "name", Type: framework.String, Required: true, Identity: true, Create: true,
			Description: "The name of the DHCP scope."},
		{Name: "hardware_address", Param: "hardwareAddress", Type: framework.String, Key: "hardwareAddress", Required: true, Identity: true, Create: true, Pattern: macAddress,
			Description: "The device's MAC address, in upper case separated by dashes, for example `00-11-22-AA-BB-CC`."},
		{Name: "ip_address", Param: "ipAddress", Type: framework.String, Key: "address", Required: true, Create: true,
			Description: "The address reserved for the device."},
		{Name: "host_name", Param: "hostName", Type: framework.String, Key: "hostName", Create: true,
			Description: "The host name to give the device instead of the one it sends."},
		{Name: "comments", Param: "comments", Type: framework.String, Key: "comments", Create: true,
			Description: "Comments stored with the reservation."},
	},
}
