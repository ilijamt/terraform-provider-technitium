//go:build integration

package examples

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"
)

func TestIntegration_ZoneAndRecords(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("records.test"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "zone_and_records", "main.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone.test", "id", "records.test"),
					resource.TestCheckResourceAttr("technitium_zone.test", "type", "Primary"),
					resource.TestCheckResourceAttr("technitium_record_a.www", "id", "records.test/www.records.test/10.0.0.1"),
					resource.TestCheckResourceAttr("technitium_record_a.www", "ttl", "300"),
					resource.TestCheckResourceAttr("technitium_record_a.www", "disable", "true"),
					resource.TestCheckResourceAttr("technitium_record_cname.app", "cname", "www.records.test"),
				),
			},
			{
				Config: ReadFixture(t, "zone_and_records", "update.tf"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_record_a.www", plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_a.www", "comments", "web"),
					resource.TestCheckResourceAttr("technitium_record_a.www", "ttl", "300"),
					resource.TestCheckResourceAttr("technitium_record_a.www", "disable", "true"),
				),
			},
			{
				Config: ReadFixture(t, "zone_and_records", "replace.tf"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_record_a.www", plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_record_a.www", "ip_address", "10.0.0.2"),
			},
			{
				ResourceName:      "technitium_record_a.www",
				ImportState:       true,
				ImportStateId:     "records.test/www.records.test/10.0.0.2",
				ImportStateVerify: true,
			},
			{
				ResourceName:      "technitium_zone.test",
				ImportState:       true,
				ImportStateId:     "records.test",
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegration_RecordRemovedOutOfBand(t *testing.T) {
	config := ReadFixture(t, "record_removed_out_of_band", "main.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("gone.test"),
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig: func() {
					_, err := api().Call(context.Background(), "/api/zones/records/delete", url.Values{
						"zone": {"gone.test"}, "domain": {"www.gone.test"}, "type": {"A"}, "ipAddress": {"10.0.0.1"},
					})
					require.NoError(t, err)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_record_a.www", plancheck.ResourceActionCreate)},
				},
				Check: func(*terraform.State) error {
					if got := records(t, "gone.test", "www.gone.test", "A"); len(got) != 1 {
						return fmt.Errorf("want the A record back, got %v", got)
					}
					return nil
				},
			},
		},
	})
}
