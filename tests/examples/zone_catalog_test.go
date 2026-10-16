//go:build integration

package examples

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"
)

func TestIntegration_ZoneLeaveCatalog(t *testing.T) {
	const dir = "zone_leave_catalog"
	member := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			opts := call(t, "/api/zones/options/get", url.Values{"zone": {"leave-member.test"}})
			if got, _ := opts["catalog"].(string); got != want {
				return fmt.Errorf("catalog %v, want %q", opts["catalog"], want)
			}
			return nil
		}
	}
	join := func() {
		call(t, "/api/zones/options/set", url.Values{"zone": {"leave-member.test"}, "catalog": {"leave-catalog.test"}})
	}
	out := ReadFixture(t, dir, "main.tf")
	in := ReadFixture(t, dir, "joined.tf")
	emptyCatalog := ReadFixture(t, dir, "empty_catalog.tf")
	empty := resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("leave-catalog.test", "leave-member.test"),
		Steps: []resource.TestStep{
			{Config: out, Check: member("")},
			{Config: out, ConfigPlanChecks: empty},
			{Config: out, ResourceName: "technitium_zone.member", ImportState: true, ImportStateVerify: true},
			{Config: in, Check: member("leave-catalog.test")},
			{Config: out, ConfigPlanChecks: expectUpdate("technitium_zone.member"), Check: member("")},
			{Config: out, ConfigPlanChecks: empty},
			{Config: in, Check: member("leave-catalog.test")},
			{Config: emptyCatalog, ConfigPlanChecks: expectUpdate("technitium_zone.member"), Check: member("")},
			{Config: emptyCatalog, ConfigPlanChecks: empty},
			{PreConfig: join, Config: out, ConfigPlanChecks: expectUpdate("technitium_zone.member"), Check: member("")},
			{Config: out, ConfigPlanChecks: empty},
		},
	})
}

// Converting a SecondaryCatalog zone to Catalog converts its members too, so
// their own convert in the same apply is skipped and the next plan is empty.
func TestIntegration_ZoneSecondaryCatalogConvert(t *testing.T) {
	const dir = "zone_secondary_catalog_convert"
	_, ip, primary := ownTestServerWith(t)
	callWith(t, primary, "/api/zones/create", url.Values{"zone": {"conv-cat.test"}, "type": {"Catalog"}})
	callWith(t, primary, "/api/zones/options/set", url.Values{"zone": {"conv-cat.test"}, "zoneTransfer": {"Allow"}})
	callWith(t, primary, "/api/zones/create", url.Values{"zone": {"conv-pri.test"}, "catalog": {"conv-cat.test"}})
	callWith(t, primary, "/api/zones/records/add", url.Values{"zone": {"conv-pri.test"}, "domain": {"www.conv-pri.test"}, "type": {"A"}, "ipAddress": {"10.0.0.1"}})
	callWith(t, primary, "/api/zones/create", url.Values{"zone": {"conv-fwd.test"}, "type": {"Forwarder"}, "catalog": {"conv-cat.test"}, "initializeForwarder": {"false"}})

	c := api()
	ctx := context.Background()
	t.Cleanup(func() {
		for _, z := range []string{"conv-cat.test", "conv-pri.test", "conv-fwd.test"} {
			_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {z}})
		}
	})

	vars := config.Variables{"ip": config.StringVariable(ip)}
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
			opts := call(t, "/api/zones/options/get", url.Values{"zone": {zone}})
			if opts["type"] != typ || fmt.Sprint(opts["catalog"]) != cat {
				return fmt.Errorf("%s is %v in catalog %v, want %s in %q", zone, opts["type"], opts["catalog"], typ, cat)
			}
			return nil
		}
	}
	secondary := ReadFixture(t, dir, "secondary.tf")
	converted := ReadFixture(t, dir, "converted.tf")
	empty := resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("conv-cat.test", "conv-pri.test", "conv-fwd.test"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, dir, "main.tf"), ConfigVariables: vars},
			{
				PreConfig:       arrived,
				Config:          secondary,
				ConfigVariables: vars,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone.pri", "type", "Secondary"),
					resource.TestCheckResourceAttr("technitium_zone.fwd", "type", "SecondaryForwarder"),
					live("conv-pri.test", "Secondary", "conv-cat.test"),
					live("conv-fwd.test", "SecondaryForwarder", "conv-cat.test"),
				),
			},
			{Config: secondary, ConfigVariables: vars, ConfigPlanChecks: empty},
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
						resp := call(t, "/api/zones/records/get", url.Values{"zone": {"conv-pri.test"}, "domain": {"www.conv-pri.test"}})
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
