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
)

// TestIntegration_AppRecord installs the Split Horizon app from the DNS App
// Store, so it needs the test server to reach the internet.
func TestIntegration_AppRecord(t *testing.T) {
	acceptance(t)
	call(t, "/api/apps/downloadAndInstall", url.Values{"name": {"Split Horizon"}, "url": {storeURL(t, "Split Horizon")}})
	t.Cleanup(func() {
		_, _ = api().Call(context.Background(), "/api/apps/uninstall", url.Values{"name": {"Split Horizon"}})
	})

	update := ReadFixture(t, "app_record", "update.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("app.test"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "app_record", "main.tf"),
				Check:  resource.TestCheckResourceAttr("technitium_record_app.www", "record_data", `{"private":["10.0.0.1"],"public":["192.0.2.1"]}`),
			},
			{
				Config:           update,
				ConfigPlanChecks: expectUpdate("technitium_record_app.www"),
				Check:            resource.TestCheckResourceAttr("technitium_record_app.www", "record_data", `{"private":["10.0.0.2"],"public":["192.0.2.1"]}`),
			},
			{
				PreConfig: func() {
					call(t, "/api/zones/records/update", url.Values{
						"zone": {"app.test"}, "domain": {"www.app.test"}, "type": {"APP"},
						"appName": {"Split Horizon"}, "classPath": {"SplitHorizon.SimpleAddress"}, "recordData": {`{"public":[],"private":[]}`},
					})
				},
				Config:           update,
				ConfigPlanChecks: expectUpdate("technitium_record_app.www"),
			},
			{
				ResourceName:      "technitium_record_app.www",
				ImportState:       true,
				ImportStateId:     "app.test/www.app.test",
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegration_Apps(t *testing.T) {
	acceptance(t)
	storeURL(t, "Split Horizon") // skips the test without the DNS App Store
	installed := func() (bool, error) {
		resp, err := api().Call(context.Background(), "/api/apps/list", nil)
		if err != nil {
			return false, err
		}
		for _, a := range resp["apps"].([]any) {
			if a.(map[string]any)["name"] == "Split Horizon" {
				return true, nil
			}
		}
		return false, nil
	}
	update := ReadFixture(t, "apps", "update.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(*terraform.State) error {
			if ok, err := installed(); err != nil || ok {
				return fmt.Errorf("app still installed (%v)", err)
			}
			return zonesDestroyed("apps.test")(nil)
		},
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "apps", "main.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("technitium_app.split_horizon", "version"),
					resource.TestCheckResourceAttrPair("technitium_app.split_horizon", "version", "data.technitium_store_app.split_horizon", "version"),
					resource.TestCheckResourceAttr("technitium_app_config.split_horizon", "config", `{"networks":{"lan":["192.168.0.0/16"]}}`),
				),
			},
			{
				Config: update,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("technitium_app_config.split_horizon", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("technitium_app.split_horizon", plancheck.ResourceActionNoop),
					},
				},
			},
			{
				PreConfig: func() {
					call(t, "/api/apps/config/set", url.Values{"name": {"Split Horizon"}, "config": {"{\r\n  \"networks\": {\r\n    \"lan\": [ \"10.0.0.0/8\" ]\r\n  }\r\n}\r\n"}})
				},
				Config:           update,
				ConfigPlanChecks: expectNoop("technitium_app_config.split_horizon"),
			},
			{
				ResourceName:            "technitium_app.split_horizon",
				ImportState:             true,
				ImportStateId:           "Split Horizon",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"url"},
			},
			{
				ResourceName:      "technitium_app_config.split_horizon",
				ImportState:       true,
				ImportStateId:     "Split Horizon",
				ImportStateVerify: true,
			},
		},
	})
}
