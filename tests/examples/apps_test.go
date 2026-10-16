//go:build integration

package examples

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/terraforming"
)

// Uninstalling an app deletes its config: installing it again starts from
// the app's default.
func TestIntegration_SideAppUninstallDropsConfig(t *testing.T) {
	acceptance(t)
	app := storeURL(t, "Split Horizon")
	c := api()
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/apps/uninstall", url.Values{"name": {"Split Horizon"}})
	})
	appConfig := func() string {
		s, _ := call(t, "/api/apps/config/get", url.Values{"name": {"Split Horizon"}})["config"].(string)
		return s
	}
	call(t, "/api/apps/downloadAndInstall", url.Values{"name": {"Split Horizon"}, "url": {app}})
	call(t, "/api/apps/config/set", url.Values{"name": {"Split Horizon"}, "config": {`{"networks":{"side":["10.9.9.0/24"]}}`}})
	require.Contains(t, appConfig(), "10.9.9.0/24")

	call(t, "/api/apps/uninstall", url.Values{"name": {"Split Horizon"}})
	call(t, "/api/apps/downloadAndInstall", url.Values{"name": {"Split Horizon"}, "url": {app}})
	assert.NotContains(t, appConfig(), "10.9.9.0/24")
}

// The app_config example: replace_triggered_by sets the config again when
// the app is replaced.
func TestIntegration_SideAppConfigReplaceTriggered(t *testing.T) {
	acceptance(t)
	app := storeURL(t, "Split Horizon")
	cfg := ReadFixture(t, "side_app_config_replace_triggered", "main.tf")
	vars := config.Variables{"url": config.StringVariable(app)}
	kept := func(*terraform.State) error {
		got, _ := call(t, "/api/apps/config/get", url.Values{"name": {"Split Horizon"}})["config"].(string)
		if !strings.Contains(got, "10.9.8.0/24") {
			return fmt.Errorf("config lost: %s", got)
		}
		return nil
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg, ConfigVariables: vars, Check: kept},
			{
				Config: cfg, ConfigVariables: vars, Taint: []string{"technitium_app.a"},
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("technitium_app_config.a", plancheck.ResourceActionDestroyBeforeCreate),
				}},
				Check: kept,
			},
		},
	})
}

func TestIntegration_AppWithoutURL(t *testing.T) {
	acceptance(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      ReadFixture(t, "app_without_url", "main.tf"),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`url is required to install an app`),
		}},
	})
}

func TestIntegration_AdoptedAppRename(t *testing.T) {
	acceptance(t)
	app := storeURL(t, "Split Horizon")
	c := api()
	call(t, "/api/apps/downloadAndInstall", url.Values{"name": {"Split Horizon"}, "url": {app}})
	t.Cleanup(func() {
		_, _ = c.Call(context.Background(), "/api/apps/uninstall", url.Values{"name": {"Split Horizon"}})
	})
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
	adopt := ReadFixture(t, "adopted_app_rename", "main.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: adopt, Check: installed},
			{Config: ReadFixture(t, "adopted_app_rename", "rename.tf"), ExpectError: regexp.MustCompile(`has no url, so renaming it`)},
			{Config: adopt, ConfigPlanChecks: expectNoop("technitium_app.x"), Check: installed},
		},
	})
}

func TestIntegration_ExportedAppNoChanges(t *testing.T) {
	acceptance(t)
	app := storeURL(t, "Split Horizon")
	c := api()
	call(t, "/api/apps/downloadAndInstall", url.Values{"name": {"Split Horizon"}, "url": {app}})
	t.Cleanup(func() {
		_, _ = c.Call(context.Background(), "/api/apps/uninstall", url.Values{"name": {"Split Horizon"}})
	})

	call(t, "/api/apps/config/set", url.Values{"name": {"Split Horizon"}, "config": {`{"networks":{"lan":["10.0.0.0/8"]},"apiKey":"s3cret"}`}})

	out := t.TempDir()
	require.NoError(t, terraforming.Run(context.Background(), terraforming.Options{
		Host: host, Token: token,
		Out: out, Version: "test", Types: []string{"app", "app_config"},
	}))
	generated := readConfig(t, out)
	require.Contains(t, generated, "# This config has keys that look like secrets (apiKey)")
	// technitium_versions.tf names ilijamt/technitium.
	t.Setenv("TF_ACC_PROVIDER_NAMESPACE", "ilijamt")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerHeader() + generated,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("technitium_app.split_horizon", plancheck.ResourceActionNoop),
					plancheck.ExpectResourceAction("technitium_app_config.split_horizon", plancheck.ResourceActionNoop),
				},
			},
		}},
	})
}
