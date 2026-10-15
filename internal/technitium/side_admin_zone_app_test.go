package technitium_test

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Uninstalling an app deletes its config: installing it again starts from
// the app's default.
func TestAccSideAppUninstallDropsConfig(t *testing.T) {
	acceptance(t)
	app := storeURL(t, "Split Horizon")
	c := api(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/apps/uninstall", url.Values{"name": {"Split Horizon"}})
	})
	config := func() string {
		s, _ := call(t, c, "/api/apps/config/get", url.Values{"name": {"Split Horizon"}})["config"].(string)
		return s
	}
	call(t, c, "/api/apps/downloadAndInstall", url.Values{"name": {"Split Horizon"}, "url": {app}})
	call(t, c, "/api/apps/config/set", url.Values{"name": {"Split Horizon"}, "config": {`{"networks":{"side":["10.9.9.0/24"]}}`}})
	require.Contains(t, config(), "10.9.9.0/24")

	call(t, c, "/api/apps/uninstall", url.Values{"name": {"Split Horizon"}})
	call(t, c, "/api/apps/downloadAndInstall", url.Values{"name": {"Split Horizon"}, "url": {app}})
	assert.NotContains(t, config(), "10.9.9.0/24")
}

// The app_config example: replace_triggered_by sets the config again when
// the app is replaced.
func TestAccSideAppConfigReplaceTriggered(t *testing.T) {
	acceptance(t)
	app := storeURL(t, "Split Horizon")
	config := fmt.Sprintf(`
resource "technitium_app" "a" {
  name = "Split Horizon"
  url  = %q
}

resource "technitium_app_config" "a" {
  name   = technitium_app.a.name
  config = jsonencode({ networks = { side = ["10.9.8.0/24"] } })

  lifecycle {
    replace_triggered_by = [technitium_app.a]
  }
}
`, app)
	kept := func(*terraform.State) error {
		got, _ := call(t, api(t), "/api/apps/config/get", url.Values{"name": {"Split Horizon"}})["config"].(string)
		if !strings.Contains(got, "10.9.8.0/24") {
			return fmt.Errorf("config lost: %s", got)
		}
		return nil
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: config, Check: kept},
			{
				Config: config, Taint: []string{"technitium_app.a"},
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("technitium_app_config.a", plancheck.ResourceActionDestroyBeforeCreate),
				}},
				Check: kept,
			},
		},
	})
}
