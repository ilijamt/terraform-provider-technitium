package technitium_test

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/testserver"
)

func names(list any, key string) []string {
	var out []string
	items, _ := list.([]any)
	for _, i := range items {
		switch v := i.(type) {
		case string:
			out = append(out, v)
		case map[string]any:
			s, _ := v[key].(string)
			out = append(out, s)
		}
	}
	return out
}

// Deleting a zone removes its per-zone permissions, and deleting a Catalog
// zone takes its members out of it.
func TestAccSideZoneDeleteDropsPermissionsAndCatalog(t *testing.T) {
	acceptance(t)
	c := api(t)
	ctx := context.Background()
	call(t, c, "/api/admin/users/create", url.Values{"user": {"sidezone"}, "pass": {"sidezone-pass"}})
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/admin/users/delete", url.Values{"user": {"sidezone"}})
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-perm.test"}})
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-member.test"}})
	})

	call(t, c, "/api/zones/create", url.Values{"zone": {"side-perm.test"}})
	call(t, c, "/api/zones/permissions/set", url.Values{"zone": {"side-perm.test"}, "userPermissions": {"sidezone|true|true|false"}})
	perms := call(t, c, "/api/zones/permissions/get", url.Values{"zone": {"side-perm.test"}})
	require.Contains(t, names(perms["userPermissions"], "username"), "sidezone")

	call(t, c, "/api/zones/delete", url.Values{"zone": {"side-perm.test"}})
	call(t, c, "/api/zones/create", url.Values{"zone": {"side-perm.test"}})
	perms = call(t, c, "/api/zones/permissions/get", url.Values{"zone": {"side-perm.test"}})
	assert.NotContains(t, names(perms["userPermissions"], "username"), "sidezone")

	call(t, c, "/api/zones/create", url.Values{"zone": {"side-catalog.test"}, "type": {"Catalog"}})
	call(t, c, "/api/zones/create", url.Values{"zone": {"side-member.test"}, "catalog": {"side-catalog.test"}})
	opts := call(t, c, "/api/zones/options/get", url.Values{"zone": {"side-member.test"}})
	require.Equal(t, "side-catalog.test", opts["catalog"])
	call(t, c, "/api/zones/delete", url.Values{"zone": {"side-catalog.test"}})
	opts = call(t, c, "/api/zones/options/get", url.Values{"zone": {"side-member.test"}})
	assert.Empty(t, opts["catalog"])
}

// The zone_permissions example: replace_triggered_by sets the permissions
// again when the zone is replaced.
func TestAccSideZonePermissionsReplaceTriggered(t *testing.T) {
	config := func(typ string) string {
		return fmt.Sprintf(`
resource "technitium_zone" "z" {
  zone = "side-replace.test"
  type = %q
}

resource "technitium_user" "u" {
  username = "sidereplace"
  password = "sidereplace-pass"
}

resource "technitium_zone_permissions" "z" {
  zone = technitium_zone.z.zone
  user_permissions = [
    { username = technitium_user.u.username, can_view = true, can_modify = true, can_delete = false },
  ]

  lifecycle {
    replace_triggered_by = [technitium_zone.z]
  }
}
`, typ)
	}
	hasUser := func(*terraform.State) error {
		perms := call(t, api(t), "/api/zones/permissions/get", url.Values{"zone": {"side-replace.test"}})
		for _, u := range names(perms["userPermissions"], "username") {
			if u == "sidereplace" {
				return nil
			}
		}
		return fmt.Errorf("sidereplace lost its rights: %v", perms["userPermissions"])
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("side-replace.test"),
		Steps: []resource.TestStep{
			{Config: config("Primary"), Check: hasUser},
			{
				Config: config("Catalog"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("technitium_zone.z", plancheck.ResourceActionDestroyBeforeCreate),
					plancheck.ExpectResourceAction("technitium_zone_permissions.z", plancheck.ResourceActionDestroyBeforeCreate),
				}},
				Check: hasUser,
			},
			{Config: config("Catalog"), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
		},
	})
}

// Members of a SecondaryCatalog zone ignore most option changes, and
// deleting the SecondaryCatalog zone deletes them.
func TestAccSideZoneSecondaryCatalogMembers(t *testing.T) {
	acceptance(t)
	if !ownServer {
		t.Skip("needs the test server in Docker to reach a second server; TECHNITIUM_TESTACC_HOST is set")
	}
	host, ip, stop, err := testserver.Run(context.Background())
	require.NoError(t, err)
	t.Cleanup(stop)
	primary, err := loginTo(host)
	require.NoError(t, err)
	call(t, primary, "/api/zones/create", url.Values{"zone": {"side-seccat.test"}, "type": {"Catalog"}})
	call(t, primary, "/api/zones/options/set", url.Values{"zone": {"side-seccat.test"}, "zoneTransfer": {"Allow"}})
	call(t, primary, "/api/zones/create", url.Values{"zone": {"side-secmember.test"}, "catalog": {"side-seccat.test"}})

	c := api(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-seccat.test"}})
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-secmember.test"}})
	})
	call(t, c, "/api/zones/create", url.Values{"zone": {"side-seccat.test"}, "type": {"SecondaryCatalog"}, "primaryNameServerAddresses": {ip}})

	var opts map[string]any
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
	call(t, c, "/api/zones/options/set", url.Values{"zone": {"side-secmember.test"}, "queryAccess": {"Deny"}, "primaryZoneTransferProtocol": {"Tls"}, "overrideCatalogQueryAccess": {"true"}})
	opts = call(t, c, "/api/zones/options/get", url.Values{"zone": {"side-secmember.test"}})
	assert.Equal(t, before, fmt.Sprint(opts["queryAccess"], opts["primaryZoneTransferProtocol"], opts["overrideCatalogQueryAccess"]), "options/set was applied")

	call(t, c, "/api/zones/delete", url.Values{"zone": {"side-seccat.test"}})
	_, err = c.Call(ctx, "/api/zones/options/get", url.Values{"zone": {"side-secmember.test"}})
	assert.Error(t, err, "member zone survived its SecondaryCatalog zone")
}
