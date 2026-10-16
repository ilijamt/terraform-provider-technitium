//go:build integration

package examples

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A new Primary zone gets an apex NS record naming the server, also one
// converted from a Forwarder zone.
func TestIntegration_SideZonePrimaryAddsNS(t *testing.T) {
	acceptance(t)
	c := api()
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-ns.test"}})
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-fwd.test"}})
	})
	ns := func(zone string) []string {
		resp := call(t, "/api/zones/records/get", url.Values{"zone": {zone}, "domain": {zone}})
		var out []string
		for _, r := range resp["records"].([]any) {
			rec := r.(map[string]any)
			if rec["type"] == "NS" {
				out = append(out, rec["rData"].(map[string]any)["nameServer"].(string))
			}
		}
		return out
	}
	general := call(t, "/api/settings/get", nil)
	server, _ := general["dnsServerDomain"].(string)
	require.NotEmpty(t, server)

	call(t, "/api/zones/create", url.Values{"zone": {"side-ns.test"}})
	assert.Equal(t, []string{server}, ns("side-ns.test"))

	call(t, "/api/zones/create", url.Values{"zone": {"side-fwd.test"}, "type": {"Forwarder"}, "initializeForwarder": {"false"}})
	call(t, "/api/zones/convert", url.Values{"zone": {"side-fwd.test"}, "type": {"Primary"}})
	assert.Equal(t, []string{server}, ns("side-fwd.test"))
}

// Deleting a zone removes its per-zone permissions, and deleting a Catalog
// zone takes its members out of it.
func TestIntegration_SideZoneDeleteDropsPermissionsAndCatalog(t *testing.T) {
	acceptance(t)
	c := api()
	ctx := context.Background()
	call(t, "/api/admin/users/create", url.Values{"user": {"sidezone"}, "pass": {"sidezone-pass"}})
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/admin/users/delete", url.Values{"user": {"sidezone"}})
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-perm.test"}})
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-member.test"}})
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-catalog.test"}})
	})

	call(t, "/api/zones/create", url.Values{"zone": {"side-perm.test"}})
	call(t, "/api/zones/permissions/set", url.Values{"zone": {"side-perm.test"}, "userPermissions": {"sidezone|true|true|false"}})
	perms := call(t, "/api/zones/permissions/get", url.Values{"zone": {"side-perm.test"}})
	require.Contains(t, names(perms["userPermissions"], "username"), "sidezone")

	call(t, "/api/zones/delete", url.Values{"zone": {"side-perm.test"}})
	call(t, "/api/zones/create", url.Values{"zone": {"side-perm.test"}})
	perms = call(t, "/api/zones/permissions/get", url.Values{"zone": {"side-perm.test"}})
	assert.NotContains(t, names(perms["userPermissions"], "username"), "sidezone")

	call(t, "/api/zones/create", url.Values{"zone": {"side-catalog.test"}, "type": {"Catalog"}})
	call(t, "/api/zones/create", url.Values{"zone": {"side-member.test"}, "catalog": {"side-catalog.test"}})
	opts := call(t, "/api/zones/options/get", url.Values{"zone": {"side-member.test"}})
	require.Equal(t, "side-catalog.test", opts["catalog"])
	call(t, "/api/zones/delete", url.Values{"zone": {"side-catalog.test"}})
	opts = call(t, "/api/zones/options/get", url.Values{"zone": {"side-member.test"}})
	assert.Empty(t, opts["catalog"])
}

// The zone_permissions example: replace_triggered_by sets the permissions
// again when the zone is replaced.
func TestIntegration_SideZonePermissionsReplaceTriggered(t *testing.T) {
	const dir = "side_zone_permissions_replace_triggered"
	hasUser := func(*terraform.State) error {
		perms := call(t, "/api/zones/permissions/get", url.Values{"zone": {"side-replace.test"}})
		for _, u := range names(perms["userPermissions"], "username") {
			if u == "sidereplace" {
				return nil
			}
		}
		return fmt.Errorf("sidereplace lost its rights: %v", perms["userPermissions"])
	}
	catalog := ReadFixture(t, dir, "catalog.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("side-replace.test"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, dir, "main.tf"), Check: hasUser},
			{
				Config: catalog,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("technitium_zone.z", plancheck.ResourceActionDestroyBeforeCreate),
					plancheck.ExpectResourceAction("technitium_zone_permissions.z", plancheck.ResourceActionDestroyBeforeCreate),
				}},
				Check: hasUser,
			},
			{Config: catalog, ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
		},
	})
}

// Members of a SecondaryCatalog zone ignore most option changes, and
// deleting the SecondaryCatalog zone deletes them.
func TestIntegration_SideZoneSecondaryCatalogMembers(t *testing.T) {
	_, ip, primary := ownTestServerWith(t)
	callWith(t, primary, "/api/zones/create", url.Values{"zone": {"side-seccat.test"}, "type": {"Catalog"}})
	callWith(t, primary, "/api/zones/options/set", url.Values{"zone": {"side-seccat.test"}, "zoneTransfer": {"Allow"}})
	callWith(t, primary, "/api/zones/create", url.Values{"zone": {"side-secmember.test"}, "catalog": {"side-seccat.test"}})

	c := api()
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-seccat.test"}})
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-secmember.test"}})
	})
	call(t, "/api/zones/create", url.Values{"zone": {"side-seccat.test"}, "type": {"SecondaryCatalog"}, "primaryNameServerAddresses": {ip}})

	var (
		opts map[string]any
		err  error
	)
	for range 120 {
		opts, err = c.Call(ctx, "/api/zones/options/get", url.Values{"zone": {"side-secmember.test"}})
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.NoError(t, err, "member zone did not arrive from the catalog")
	require.Equal(t, "Secondary", opts["type"])
	require.Equal(t, "side-seccat.test", opts["catalog"])

	before := fmt.Sprint(opts["queryAccess"], opts["primaryZoneTransferProtocol"], opts["overrideCatalogQueryAccess"])
	call(t, "/api/zones/options/set", url.Values{"zone": {"side-secmember.test"}, "queryAccess": {"Deny"}, "primaryZoneTransferProtocol": {"Tls"}, "overrideCatalogQueryAccess": {"true"}})
	opts = call(t, "/api/zones/options/get", url.Values{"zone": {"side-secmember.test"}})
	assert.Equal(t, before, fmt.Sprint(opts["queryAccess"], opts["primaryZoneTransferProtocol"], opts["overrideCatalogQueryAccess"]), "options/set was applied")

	call(t, "/api/zones/delete", url.Values{"zone": {"side-seccat.test"}})
	_, err = c.Call(ctx, "/api/zones/options/get", url.Values{"zone": {"side-secmember.test"}})
	assert.Error(t, err, "member zone survived its SecondaryCatalog zone")
}

// The server drops users and groups it does not know from a permissions
// list without an error, so the provider reports them.
func TestIntegration_SidePermissionsUnknownNames(t *testing.T) {
	const dir = "side_permissions_unknown_names"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("side-unknown.test"),
		Steps: []resource.TestStep{
			{
				Config:      ReadFixture(t, dir, "main.tf"),
				ExpectError: regexp.MustCompile(`zone_permissions: user "nosuchuser" does not exist`),
			},
			{
				Config:      ReadFixture(t, dir, "logs.tf"),
				ExpectError: regexp.MustCompile(`permissions_logs: group "No Such Group" does not\s+exist`),
			},
		},
	})
}

// The server stores an IP address under its reverse name, and adding an
// entry again succeeds without a change.
func TestIntegration_SideListedZoneServer(t *testing.T) {
	acceptance(t)
	c := api()
	ctx := context.Background()
	for _, kind := range []string{"allowed", "blocked"} {
		t.Cleanup(func() {
			_, _ = c.Call(ctx, "/api/"+kind+"/delete", url.Values{"domain": {"5.1.168.192.in-addr.arpa"}})
			_, _ = c.Call(ctx, "/api/"+kind+"/delete", url.Values{"domain": {"side-listed.test"}})
		})
		call(t, "/api/"+kind+"/add", url.Values{"domain": {"192.168.1.5"}})
		resp := call(t, "/api/"+kind+"/list", url.Values{"domain": {"5.1.168.192.in-addr.arpa"}})
		assert.Equal(t, "5.1.168.192.in-addr.arpa", resp["domain"], kind)
		assert.NotEmpty(t, resp["records"], kind)

		call(t, "/api/"+kind+"/add", url.Values{"domain": {"side-listed.test"}})
		call(t, "/api/"+kind+"/add", url.Values{"domain": {"side-listed.test"}})
	}
}

// IP addresses and networks are refused at plan time, and an existing entry
// on create.
func TestIntegration_SideListedZoneRefused(t *testing.T) {
	acceptance(t)
	const dir = "side_listed_zone_refused"
	c := api()
	ctx := context.Background()
	call(t, "/api/blocked/add", url.Values{"domain": {"side-existing.test"}})
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/blocked/delete", url.Values{"domain": {"side-existing.test"}})
	})
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:      ReadFixture(t, dir, "ip.tf"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Domain is an IP address`),
			},
			{
				Config:      ReadFixture(t, dir, "network.tf"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`10.in-addr.arpa`),
			},
			{
				Config:      ReadFixture(t, dir, "existing.tf"),
				ExpectError: regexp.MustCompile(`blocked_zone already exists; import it`),
			},
			{
				Config: ReadFixture(t, dir, "sub.tf"),
				Check:  resource.TestCheckResourceAttr("technitium_blocked_zone.sub", "domain", "sub.side-existing.test"),
			},
		},
	})
}
