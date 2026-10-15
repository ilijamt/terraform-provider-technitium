package technitium

import (
	"context"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
)

// zoneNameValidator refuses IP addresses and networks: the server names the
// zone after the reverse name instead, and the zone could not be read back.
type zoneNameValidator struct{}

func (zoneNameValidator) Description(context.Context) string {
	return "must be a domain name, not an IP address or network"
}

func (v zoneNameValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (zoneNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if hint := reverseZoneHint(req.ConfigValue.ValueString()); hint != "" {
		resp.Diagnostics.AddAttributeError(req.Path, "Zone name is an IP address",
			"The server names a zone given as an IP address or network after its reverse name: "+hint+".")
	}
}

// partialIP is what .NET still parses as an IPv4 address, such as "10.1".
var partialIP = regexp.MustCompile(`^[0-9.]*[0-9][0-9.]*$`)

func reverseZoneHint(name string) string {
	if addr, err := netip.ParseAddr(name); err == nil {
		return fmt.Sprintf("use the reverse zone name %q for %s", reverseZone(addr, addr.BitLen()), name)
	}
	if p, err := netip.ParsePrefix(name); err == nil {
		p = p.Masked()
		step := 4
		if p.Addr().Is4() {
			step = 8
		}
		bits := p.Bits() / step * step
		if bits == p.Bits() {
			return fmt.Sprintf("use the reverse zone name %q for %s", reverseZone(p.Addr(), bits), p)
		}
		outer := netip.PrefixFrom(p.Addr(), bits).Masked()
		return fmt.Sprintf("use the reverse zone name %q for %s, or one zone per /%d such as %q",
			reverseZone(outer.Addr(), bits), outer, bits+step, reverseZone(p.Addr(), bits+step))
	}
	if partialIP.MatchString(name) {
		return fmt.Sprintf("the server reads %s as an IP address; use its reverse zone name", name)
	}
	return ""
}

func reverseZone(addr netip.Addr, bits int) string {
	var labels []string
	b := addr.AsSlice()
	if addr.Is4() {
		for i := range bits / 8 {
			labels = append(labels, fmt.Sprint(b[i]))
		}
		slices.Reverse(labels)
		return strings.Join(append(labels, "in-addr", "arpa"), ".")
	}
	for i := range bits / 4 {
		n := b[i/2] >> 4
		if i%2 == 1 {
			n = b[i/2] & 0xf
		}
		labels = append(labels, fmt.Sprintf("%x", n))
	}
	slices.Reverse(labels)
	return strings.Join(append(labels, "ip6", "arpa"), ".")
}

var zoneConvert = framework.Convert{
	Path: "/api/zones/convert",
	From: map[string][]string{
		"Primary":            {"Forwarder"},
		"Forwarder":          {"Primary"},
		"Secondary":          {"Primary", "Forwarder", "Catalog"},
		"SecondaryForwarder": {"Primary", "Forwarder", "Catalog"},
		"SecondaryCatalog":   {"Primary", "Forwarder", "Catalog"},
	},
}

var zone = framework.Spec{
	Name:        "zone",
	Description: "Manages an authoritative zone.",
	Create:      "/api/zones/create",
	Read:        "/api/zones/options/get",
	Update:      "/api/zones/options/set",
	Delete:      "/api/zones/delete",
	// A Forwarder zone starts empty; its forwarders are technitium_record_fwd
	// resources at the zone apex.
	CreateFixed: map[string]string{"initializeForwarder": "false"},
	Attrs: []framework.Attr{
		{Name: "zone", Param: "zone", Type: framework.String, Key: "name", Required: true, Identity: true, Create: true, Update: true,
			Validators:  []validator.String{zoneNameValidator{}},
			Description: "The zone name. A reverse zone takes its `in-addr.arpa` or `ip6.arpa` name, for example `1.168.192.in-addr.arpa` for `192.168.1.0/24`. An IP address or network is refused, as the server would create the zone under the reverse name."},
		{Name: "type", Param: "type", Type: framework.String, Key: "type", Create: true, Convert: &zoneConvert, Enum: []string{"Primary", "Secondary", "Stub", "Forwarder", "SecondaryForwarder", "Catalog", "SecondaryCatalog"},
			Description: "The zone type: `Primary` (default), `Secondary`, `Stub`, `Forwarder`, `SecondaryForwarder`, `Catalog` or `SecondaryCatalog`. A `Forwarder` zone starts empty: add its forwarders as `technitium_record_fwd` resources with `domain` set to the zone name. A `Primary` zone converts in place to `Forwarder` and back, and a `Secondary`, `SecondaryForwarder` or `SecondaryCatalog` zone to `Primary`, `Forwarder` or `Catalog`, keeping its records except those the new type lacks: a `Forwarder` zone has no SOA or NS records, and a `Primary` zone no FWD records. The zone options take the new type's defaults unless configured. A signed zone cannot become a `Forwarder`. Converting a `SecondaryCatalog` zone to `Catalog` also converts its members: a `SecondaryForwarder` member becomes a `Forwarder`, and a `Secondary` member that is a `Primary` zone on the catalog's primary becomes a `Primary`, signed with its keys if it was signed. Set the new `type` on member zones managed as `technitium_zone` in the same apply, and set their `catalog` to a reference to the catalog zone so they update after it: a member the catalog already converted is not converted again. Any other change replaces the zone, which deletes all its records; the record resources in the zone plan no change until the next apply, which creates them again."},
		{Name: "catalog", Param: "catalog", Type: framework.String, Key: "catalog", Create: true, Update: true, Compare: framework.ZoneName, Default: "",
			Resets:      []string{"override_catalog_query_access", "override_catalog_zone_transfer", "override_catalog_notify"},
			Description: "The catalog zone this zone is a member of. Leaving it unset or empty takes the zone out of its catalog, also when it joined one outside Terraform. Applies to `Primary`, `Secondary`, `Stub` and `Forwarder` zones. A member of a `SecondaryCatalog` zone cannot leave it: set `catalog` to that zone's name."},
		{Name: "primary_name_server_addresses", Param: "primaryNameServerAddresses", Type: framework.List, Key: "primaryNameServerAddresses", Create: true, Update: true,
			Description: "The IP addresses or domain names of the primary name servers. Applies to `Secondary`, `SecondaryForwarder`, `SecondaryCatalog` and `Stub` zones, and is required for `SecondaryForwarder` and `SecondaryCatalog`. When unset, `Secondary` and `Stub` zones resolve them through the zone's name servers. With `primary_zone_transfer_protocol` set to `Tls` or `Quic`, include the port, for example `192.0.2.1:853`: the server adds it otherwise, which Terraform reports as an inconsistent result."},
		{Name: "disabled", Param: "disabled", Type: framework.Bool, Key: "disabled", Update: true,
			Description: "Disable the zone without deleting it."},
		{Name: "override_catalog_query_access", Param: "overrideCatalogQueryAccess", Type: framework.Bool, Key: "overrideCatalogQueryAccess", Update: true,
			Description: "Use this zone's `query_access` instead of the catalog zone's. Applies to `Primary`, `Secondary`, `Stub` and `Forwarder` zones."},
		{Name: "override_catalog_zone_transfer", Param: "overrideCatalogZoneTransfer", Type: framework.Bool, Key: "overrideCatalogZoneTransfer", Update: true,
			Description: "Use this zone's `zone_transfer` instead of the catalog zone's. Applies to `Primary`, `Secondary` and `Forwarder` zones."},
		{Name: "override_catalog_notify", Param: "overrideCatalogNotify", Type: framework.Bool, Key: "overrideCatalogNotify", Update: true,
			Description: "Use this zone's `notify` instead of the catalog zone's. Applies to `Primary` and `Forwarder` zones."},
		{Name: "primary_zone_transfer_protocol", Param: "primaryZoneTransferProtocol", CreateParam: "zoneTransferProtocol", Type: framework.String, Key: "primaryZoneTransferProtocol", Create: true, Update: true, Enum: []string{"Tcp", "Tls", "Quic"},
			Description: "The protocol for transferring the zone from its primary: `Tcp`, `Tls` or `Quic`. Applies to `Secondary`, `SecondaryForwarder` and `SecondaryCatalog` zones."},
		{Name: "primary_zone_transfer_tsig_key_name", Param: "primaryZoneTransferTsigKeyName", CreateParam: "tsigKeyName", Type: framework.String, Key: "primaryZoneTransferTsigKeyName", Create: true, Update: true,
			Description: "The TSIG key for transferring the zone from its primary, one of the keys in `technitium_settings_tsig`. Applies to `Secondary`, `SecondaryForwarder` and `SecondaryCatalog` zones."},
		{Name: "query_access", Param: "queryAccess", Type: framework.String, Key: "queryAccess", Update: true, Enum: []string{"Deny", "Allow", "AllowOnlyPrivateNetworks", "AllowOnlyZoneNameServers", "UseSpecifiedNetworkACL", "AllowZoneNameServersAndUseSpecifiedNetworkACL"},
			Description: "Who may query the zone: `Deny`, `Allow`, `AllowOnlyPrivateNetworks`, `AllowOnlyZoneNameServers`, `UseSpecifiedNetworkACL` or `AllowZoneNameServersAndUseSpecifiedNetworkACL`."},
		{Name: "query_access_network_acl", Param: "queryAccessNetworkACL", Type: framework.List, Key: "queryAccessNetworkACL", Update: true,
			Description: "The networks that may query the zone when `query_access` uses a network ACL. Each entry is an IP address or network; prefix it with `!` to deny it. Entries are checked in order, and anything unmatched is denied except loopback. An empty list clears it. Applies to all zones except `SecondaryCatalog`."},
		{Name: "zone_transfer", Param: "zoneTransfer", Type: framework.String, Key: "zoneTransfer", Update: true, Enum: []string{"Deny", "Allow", "AllowOnlyZoneNameServers", "UseSpecifiedNetworkACL", "AllowZoneNameServersAndUseSpecifiedNetworkACL"},
			Description: "Who may transfer the zone: `Deny`, `Allow`, `AllowOnlyZoneNameServers`, `UseSpecifiedNetworkACL` or `AllowZoneNameServersAndUseSpecifiedNetworkACL`. Applies to `Primary` and `Secondary` zones."},
		{Name: "zone_transfer_network_acl", Param: "zoneTransferNetworkACL", Type: framework.List, Key: "zoneTransferNetworkACL", Update: true,
			Description: "The networks that may transfer the zone when `zone_transfer` uses a network ACL. Each entry is an IP address or network; prefix it with `!` to deny it. Entries are checked in order, and anything unmatched is denied. An empty list clears it. Applies to `Primary`, `Secondary`, `Forwarder` and `Catalog` zones."},
		{Name: "zone_transfer_tsig_key_names", Param: "zoneTransferTsigKeyNames", Type: framework.List, Key: "zoneTransferTsigKeyNames", Update: true,
			Description: "The TSIG keys allowed to transfer the zone. An empty list clears it. Applies to `Primary`, `Secondary`, `Forwarder` and `Catalog` zones."},
		{Name: "notify", Param: "notify", Type: framework.String, Key: "notify", Update: true,
			Description: "Which name servers to notify of zone changes. `Primary` and `Secondary` zones accept `None`, `ZoneNameServers`, `SpecifiedNameServers` or `BothZoneAndSpecifiedNameServers`. `Forwarder` and `Catalog` zones accept `None` or `SpecifiedNameServers`, and `Catalog` zones also `SeparateNameServersForCatalogAndMemberZones`."},
		{Name: "notify_name_servers", Param: "notifyNameServers", Type: framework.List, Key: "notifyNameServers", Update: true,
			Description: "The IP addresses to notify of zone changes when `notify` is `SpecifiedNameServers` or `BothZoneAndSpecifiedNameServers`. Applies to `Primary`, `Secondary`, `Forwarder` and `Catalog` zones."},
		{Name: "notify_secondary_catalogs_name_servers", Param: "notifySecondaryCatalogsNameServers", Type: framework.List, Key: "notifySecondaryCatalogsNameServers", Update: true,
			Description: "The IP addresses to notify of catalog zone changes only, when `notify` is `SeparateNameServersForCatalogAndMemberZones`. Applies to `Catalog` zones."},
		{Name: "update", Param: "update", Type: framework.String, Key: "update", Update: true,
			Description: "Who may send dynamic updates (RFC 2136). `Primary` zones accept `Deny`, `Allow`, `AllowOnlyZoneNameServers`, `UseSpecifiedNetworkACL` or `AllowZoneNameServersAndUseSpecifiedNetworkACL`. `Secondary` and `Forwarder` zones accept `Deny`, `Allow` or `UseSpecifiedNetworkACL`."},
		{Name: "update_network_acl", Param: "updateNetworkACL", Type: framework.List, Key: "updateNetworkACL", Update: true,
			Description: "The networks that may send dynamic updates when `update` uses a network ACL. Each entry is an IP address or network; prefix it with `!` to deny it. Entries are checked in order, and anything unmatched is denied. An empty list clears it. Applies to `Primary`, `Secondary` and `Forwarder` zones."},
		{Name: "update_security_policies", Param: "updateSecurityPolicies", Type: framework.Table, Key: "updateSecurityPolicies", Update: true,
			Columns: []framework.Column{
				{Name: "tsig_key_name", Key: "tsigKeyName", Type: framework.String,
					Description: "The TSIG key, one of the keys in `technitium_settings_tsig`, in lower case."},
				{Name: "domain", Key: "domain", Type: framework.String,
					Description: "The domain name in the zone the key may change, in lower case. A wildcard such as `*.example.com` covers all its subdomains."},
				{Name: "allowed_types", Key: "allowedTypes", Type: framework.List,
					Description: "The record types the key may change, in upper case, for example `A` and `AAAA`."},
			},
			Description: "Which records each TSIG key may change through dynamic updates, one entry per key and domain. An empty set removes the policies and turns off TSIG authentication for updates. Applies to `Primary` and `Forwarder` zones."},
	},
}
