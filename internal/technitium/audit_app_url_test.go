package technitium_test

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccAppWithoutURL(t *testing.T) {
	acceptance(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: `
resource "technitium_app" "x" {
  name = "Not Installed"
}
`,
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`url is required to install an app`),
		}},
	})
}

func TestAccAdoptedAppRename(t *testing.T) {
	acceptance(t)
	app := storeURL(t, "Split Horizon")
	c := api(t)
	call(t, c, "/api/apps/downloadAndInstall", url.Values{"name": {"Split Horizon"}, "url": {app}})
	t.Cleanup(func() {
		_, _ = c.Call(context.Background(), "/api/apps/uninstall", url.Values{"name": {"Split Horizon"}})
	})
	config := func(name string) string {
		return fmt.Sprintf(`
import {
  to = technitium_app.x
  id = "Split Horizon"
}

resource "technitium_app" "x" {
  name = %q
}
`, name)
	}
	installed := func(*terraform.State) error {
		resp, err := c.Call(context.Background(), "/api/apps/list", nil)
		if err != nil {
			return err
		}
		for _, a := range resp["apps"].([]any) {
			if a.(map[string]any)["name"] == "Split Horizon" {
				return nil
			}
		}
		return fmt.Errorf("Split Horizon was uninstalled")
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: config("Split Horizon"), Check: installed},
			{Config: config("Other App"), ExpectError: regexp.MustCompile(`has no url, so renaming it`)},
			{
				Config:           config("Split Horizon"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_app.x", plancheck.ResourceActionNoop)}},
				Check:            installed,
			},
		},
	})
}
