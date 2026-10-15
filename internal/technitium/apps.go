package technitium

import (
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
)

// The server does not return the URL, so the configured one is kept. It is
// optional so an imported app can leave it out; setting it then updates the
// app from it.
var app = framework.Spec{
	Name:        "app",
	Description: "Installs a DNS app from a download URL, such as one from the DNS App Store. Changing the URL updates the app in place, keeping its config; destroying or replacing the resource uninstalls it, which deletes its config. Add `replace_triggered_by` on its `technitium_app_config` to set the config again in the same apply.",
	Create:      "/api/apps/downloadAndInstall",
	Read:        "/api/apps/list",
	Update:      "/api/apps/downloadAndUpdate",
	Delete:      "/api/apps/uninstall",
	List:        "apps",
	Check:       checkApp,
	Attrs: []framework.Attr{
		{Name: "name", Param: "name", Type: framework.String, Key: "name", Required: true, Identity: true, Create: true, Update: true,
			Description: "The app name, for example `Split Horizon`. Changing it replaces the app, which needs `url` to install the new one."},
		{Name: "url", Param: "url", Type: framework.String, Create: true, Update: true, Pattern: `^https://`,
			Description: "The `https://` URL of the app's zip file, for example `https://download.technitium.com/dns/apps/SplitHorizonApp-v11.zip`. Needed to install the app; an imported app may leave it out, and setting or changing it updates the app from that URL. The server does not return it, so an app updated outside Terraform is seen through `version`."},
		{Name: "version", Param: "version", Type: framework.String, Key: "version", ReadOnly: true,
			Description: "The installed version."},
		{Name: "update_available", Param: "updateAvailable", Type: framework.Bool, Key: "updateAvailable", ReadOnly: true,
			Description: "Whether the DNS App Store has a newer version."},
		{Name: "update_version", Param: "updateVersion", Type: framework.String, Key: "updateVersion", ReadOnly: true,
			Description: "The newest version in the DNS App Store."},
	},
}

// checkApp refuses an install without a URL, including the one that
// replaces an adopted app on a rename, which would uninstall it first.
func checkApp(prior, plan map[string]tftypes.Value) error {
	if !plan["url"].IsNull() {
		return nil
	}
	if prior == nil {
		return errors.New("url is required to install an app; to manage an installed app, import it")
	}
	if !plan["name"].IsKnown() || plan["name"].Equal(prior["name"]) {
		return nil
	}
	var from, to string
	_ = prior["name"].As(&from)
	_ = plan["name"].As(&to)
	return fmt.Errorf("%s has no url, so renaming it to %s would uninstall it and fail to install %s; set url to replace it, or import %s instead", from, to, to, to)
}

var appConfig = framework.Spec{
	Name:        "app_config",
	Description: "Manages the config of an installed DNS app. The format is the app's own, usually JSON. Destroying the resource leaves the config as it is. Uninstalling or replacing the app deletes its config: add `replace_triggered_by` on the `technitium_app` to set it again in the same apply.",
	Read:        "/api/apps/config/get",
	Update:      "/api/apps/config/set",
	Attrs: []framework.Attr{
		{Name: "name", Param: "name", Type: framework.String, Required: true, Identity: true, Update: true,
			Description: "The name of the installed app."},
		{Name: "config", Param: "config", Type: framework.String, Key: "config", Required: true, Update: true, JSON: true, Sensitive: true,
			Description: "The app's config as JSON, for example from `jsonencode(...)`. Differences in spacing and key order alone are not changes."},
	},
}

var storeApp = framework.Spec{
	Name:        "store_app",
	Description: "An app in the DNS App Store.",
	Read:        "/api/apps/listStoreApps",
	List:        "storeApps",
	Attrs: []framework.Attr{
		{Name: "name", Param: "name", Type: framework.String, Key: "name", Required: true, Identity: true,
			Description: "The app name, for example `Split Horizon`."},
		{Name: "url", Param: "url", Type: framework.String, Key: "url",
			Description: "The download URL of the newest version, for `technitium_app`."},
		{Name: "version", Param: "version", Type: framework.String, Key: "version",
			Description: "The newest version."},
		{Name: "size", Param: "size", Type: framework.String, Key: "size",
			Description: "The size of the download, for example `61.57 KB`."},
		{Name: "description", Param: "description", Type: framework.String, Key: "description",
			Description: "What the app does."},
		{Name: "installed", Param: "installed", Type: framework.Bool, Key: "installed",
			Description: "Whether the app is installed on the server."},
	},
}
