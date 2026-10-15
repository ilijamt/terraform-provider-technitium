// Package technitium holds the resource specs.
package technitium

import (
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
)

// Specs returns the resource specs.
func Specs() []framework.Spec {
	specs := append([]framework.Spec{zone, recordSOA}, records...)
	specs = append(specs, dhcpScope, dhcpReservedLease, allowedZone, blockedZone, user, group, groupMembers)
	specs = append(specs, permissions...)
	return append(specs, settings...)
}

// dataSources are the specs that also get a data source: the ones worth
// reading without managing them. A single record is not, as finding one takes
// all its data.
var dataSources = []string{"zone", "record_soa", "dhcp_scope", "dhcp_reserved_lease", "user", "group", "group_members"}

// DataSources returns a data source for the specs in dataSources, every
// settings area and every permissions section.
func DataSources() []func() datasource.DataSource {
	var out []func() datasource.DataSource
	for _, s := range Specs() {
		if slices.Contains(dataSources, s.Name) || strings.HasPrefix(s.Name, "settings_") || strings.HasPrefix(s.Name, "permissions_") {
			out = append(out, framework.NewDataSource(s))
		}
	}
	return out
}

func Resources() []func() resource.Resource {
	var out []func() resource.Resource
	for _, s := range Specs() {
		out = append(out, framework.NewResource(s))
	}
	return out
}
