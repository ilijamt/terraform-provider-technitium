package technitium_test

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/terraforming"
	"github.com/ilijamt/terraform-provider-technitium/internal/testserver"
)

func TestAccExportedAppNoChanges(t *testing.T) {
	acceptance(t)
	app := storeURL(t, "Split Horizon")
	c := api(t)
	call(t, c, "/api/apps/downloadAndInstall", url.Values{"name": {"Split Horizon"}, "url": {app}})
	t.Cleanup(func() {
		_, _ = c.Call(context.Background(), "/api/apps/uninstall", url.Values{"name": {"Split Horizon"}})
	})

	call(t, c, "/api/apps/config/set", url.Values{"name": {"Split Horizon"}, "config": {`{"networks":{"lan":["10.0.0.0/8"]},"apiKey":"s3cret"}`}})

	out := t.TempDir()
	require.NoError(t, terraforming.Run(context.Background(), terraforming.Options{
		Host: os.Getenv("TECHNITIUM_HOST"), Username: testserver.Username, Password: testserver.Password,
		Out: out, Version: "test", Types: []string{"app", "app_config"},
	}))
	require.Contains(t, readConfig(t, out), "# This config has keys that look like secrets (apiKey)")
	// technitium_versions.tf names ilijamt/technitium.
	t.Setenv("TF_ACC_PROVIDER_NAMESPACE", "ilijamt")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: readConfig(t, out),
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("technitium_app.split_horizon", plancheck.ResourceActionNoop),
					plancheck.ExpectResourceAction("technitium_app_config.split_horizon", plancheck.ResourceActionNoop),
				},
			},
		}},
	})
}
