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
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/testserver"
)

func TestAccZoneLeaveCatalog(t *testing.T) {
	config := func(catalog string) string {
		return fmt.Sprintf(`
resource "technitium_zone" "catalog" {
  zone = "leave-catalog.test"
  type = "Catalog"
}

resource "technitium_zone" "member" {
  zone       = "leave-member.test"
  %s
  depends_on = [technitium_zone.catalog]
}
`, catalog)
	}
	member := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			opts := call(t, api(t), "/api/zones/options/get", url.Values{"zone": {"leave-member.test"}})
			if got, _ := opts["catalog"].(string); got != want {
				return fmt.Errorf("catalog %v, want %q", opts["catalog"], want)
			}
			return nil
		}
	}
	join := func() {
		call(t, api(t), "/api/zones/options/set", url.Values{"zone": {"leave-member.test"}, "catalog": {"leave-catalog.test"}})
	}
	in := config(`catalog = "leave-catalog.test"`)
	empty := resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("leave-catalog.test", "leave-member.test"),
		Steps: []resource.TestStep{
			{Config: config(""), Check: member("")},
			{Config: config(""), ConfigPlanChecks: empty},
			{Config: config(""), ResourceName: "technitium_zone.member", ImportState: true, ImportStateVerify: true},
			{Config: in, Check: member("leave-catalog.test")},
			{Config: config(""), ConfigPlanChecks: expectUpdate("technitium_zone.member"), Check: member("")},
			{Config: config(""), ConfigPlanChecks: empty},
			{Config: in, Check: member("leave-catalog.test")},
			{Config: config(`catalog = ""`), ConfigPlanChecks: expectUpdate("technitium_zone.member"), Check: member("")},
			{Config: config(`catalog = ""`), ConfigPlanChecks: empty},
			{PreConfig: join, Config: config(""), ConfigPlanChecks: expectUpdate("technitium_zone.member"), Check: member("")},
			{Config: config(""), ConfigPlanChecks: empty},
		},
	})
}

// Converting a SecondaryCatalog zone to Catalog converts its members too, so
// their own convert in the same apply is skipped and the next plan is empty.
func TestAccZoneSecondaryCatalogConvert(t *testing.T) {
	acceptance(t)
	if !ownServer {
		t.Skip("needs the test server in Docker to reach a second server; TECHNITIUM_TESTACC_HOST is set")
	}
	host, ip, stop, err := testserver.Run(context.Background())
	require.NoError(t, err)
	t.Cleanup(stop)
	primary, err := loginTo(host)
	require.NoError(t, err)
	call(t, primary, "/api/zones/create", url.Values{"zone": {"conv-cat.test"}, "type": {"Catalog"}})
	call(t, primary, "/api/zones/options/set", url.Values{"zone": {"conv-cat.test"}, "zoneTransfer": {"Allow"}})
	call(t, primary, "/api/zones/create", url.Values{"zone": {"conv-pri.test"}, "catalog": {"conv-cat.test"}})
	call(t, primary, "/api/zones/records/add", url.Values{"zone": {"conv-pri.test"}, "domain": {"www.conv-pri.test"}, "type": {"A"}, "ipAddress": {"10.0.0.1"}})
	call(t, primary, "/api/zones/create", url.Values{"zone": {"conv-fwd.test"}, "type": {"Forwarder"}, "catalog": {"conv-cat.test"}, "initializeForwarder": {"false"}})

	c := api(t)
	ctx := context.Background()
	t.Cleanup(func() {
		for _, z := range []string{"conv-cat.test", "conv-pri.test", "conv-fwd.test"} {
			_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {z}})
		}
	})

	catalog := func(typ string) string {
		primaries := ""
		if typ == "SecondaryCatalog" {
			primaries = fmt.Sprintf("primary_name_server_addresses = [%q]", ip)
		}
		return fmt.Sprintf(`
resource "technitium_zone" "cat" {
  zone = "conv-cat.test"
  type = %q
  %s
}
`, typ, primaries)
	}
	members := func(pri, fwd string) string {
		return fmt.Sprintf(`
import {
  to = technitium_zone.pri
  id = "conv-pri.test"
}

import {
  to = technitium_zone.fwd
  id = "conv-fwd.test"
}

resource "technitium_zone" "pri" {
  zone    = "conv-pri.test"
  type    = %q
  catalog = technitium_zone.cat.zone
}

resource "technitium_zone" "fwd" {
  zone    = "conv-fwd.test"
  type    = %q
  catalog = technitium_zone.cat.zone
}
`, pri, fwd)
	}
	arrived := func() {
		for _, z := range []string{"conv-pri.test", "conv-fwd.test"} {
			var err error
			for range 120 {
				if _, err = c.Call(ctx, "/api/zones/options/get", url.Values{"zone": {z}}); err == nil {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
			require.NoError(t, err, "%s did not arrive from the catalog", z)
		}
	}
	live := func(zone, typ, cat string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			opts := call(t, c, "/api/zones/options/get", url.Values{"zone": {zone}})
			if opts["type"] != typ || fmt.Sprint(opts["catalog"]) != cat {
				return fmt.Errorf("%s is %v in catalog %v, want %s in %q", zone, opts["type"], opts["catalog"], typ, cat)
			}
			return nil
		}
	}
	secondary := catalog("SecondaryCatalog") + members("Secondary", "SecondaryForwarder")
	converted := catalog("Catalog") + members("Primary", "Forwarder")
	empty := resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("conv-cat.test", "conv-pri.test", "conv-fwd.test"),
		Steps: []resource.TestStep{
			{Config: catalog("SecondaryCatalog")},
			{
				PreConfig: arrived,
				Config:    secondary,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone.pri", "type", "Secondary"),
					resource.TestCheckResourceAttr("technitium_zone.fwd", "type", "SecondaryForwarder"),
					live("conv-pri.test", "Secondary", "conv-cat.test"),
					live("conv-fwd.test", "SecondaryForwarder", "conv-cat.test"),
				),
			},
			{Config: secondary, ConfigPlanChecks: empty},
			{
				Config: converted,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("technitium_zone.cat", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("technitium_zone.pri", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("technitium_zone.fwd", plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeTestCheckFunc(
					live("conv-cat.test", "Catalog", "<nil>"),
					live("conv-pri.test", "Primary", "conv-cat.test"),
					live("conv-fwd.test", "Forwarder", "conv-cat.test"),
					func(*terraform.State) error {
						resp := call(t, c, "/api/zones/records/get", url.Values{"zone": {"conv-pri.test"}, "domain": {"www.conv-pri.test"}})
						if len(resp["records"].([]any)) == 0 {
							return fmt.Errorf("www.conv-pri.test was lost in the conversion")
						}
						return nil
					},
				),
			},
			{Config: converted, ConfigPlanChecks: empty},
			{Config: converted, ResourceName: "technitium_zone.pri", ImportState: true, ImportStateVerify: true},
			{Config: converted, ResourceName: "technitium_zone.fwd", ImportState: true, ImportStateVerify: true},
		},
	})
}
