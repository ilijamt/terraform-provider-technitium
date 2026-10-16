//go:build integration

package examples

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestIntegration_ZoneDNSSEC(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("signed.test"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "zone_dnssec", "main.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "status", "SignedWithNSEC3"),
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "nsec3_iterations", "0"),
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "keys.#", "2"),
					resource.TestCheckResourceAttrSet("technitium_zone_dnssec.test", "ds_records.0.digest"),
					resource.TestCheckResourceAttr("data.technitium_zone_dnssec.test", "signed", "true"),
					resource.TestCheckResourceAttr("data.technitium_zone_dnssec.test", "curve", "P256"),
				),
			},
			{
				Config: ReadFixture(t, "zone_dnssec", "update.tf"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_zone_dnssec.test", plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "status", "SignedWithNSEC"),
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "zsk_rollover_days", "60"),
					resource.TestCheckNoResourceAttr("technitium_zone_dnssec.test", "nsec3_iterations"),
				),
			},
			{
				ResourceName:      "technitium_zone_dnssec.test",
				ImportState:       true,
				ImportStateId:     "signed.test",
				ImportStateVerify: true,
			},
			{
				Config: ReadFixture(t, "zone_dnssec", "unsigned.tf"),
				Check: func(*terraform.State) error {
					resp := call(t, "/api/zones/dnssec/properties/get", url.Values{"zone": {"signed.test"}})
					if resp["dnssecStatus"] != "Unsigned" {
						return fmt.Errorf("zone is %v after destroy", resp["dnssecStatus"])
					}
					return nil
				},
			},
		},
	})
}
