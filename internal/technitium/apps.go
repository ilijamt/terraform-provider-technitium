package technitium

import "github.com/ilijamt/terraform-provider-technitium/internal/framework"

// app is an installed DNS app, downloaded from a URL such as the DNS App
// Store's. The server does not return the URL, so it keeps the configured one.
var app = framework.Spec{
	Name:        "app",
	Description: "Installs a DNS app from a download URL, such as one from the DNS App Store. Changing the URL updates the app in place; destroying the resource uninstalls it.",
	Create:      "/api/apps/downloadAndInstall",
	Read:        "/api/apps/list",
	Update:      "/api/apps/downloadAndUpdate",
	Delete:      "/api/apps/uninstall",
	List:        "apps",
	Attrs: []framework.Attr{
		{Name: "name", Param: "name", Type: framework.String, Key: "name", Required: true, Identity: true, Create: true, Update: true,
			Description: "The app name, for example `Split Horizon`. Changing it replaces the app."},
		{Name: "url", Param: "url", Type: framework.String, Required: true, Create: true, Update: true, Pattern: `^https://`,
			Description: "The `https://` URL of the app's zip file, for example `https://download.technitium.com/dns/apps/SplitHorizonApp-v11.zip`. The server does not return it, so an app updated outside Terraform is seen through `version`."},
		{Name: "version", Param: "version", Type: framework.String, Key: "version", ReadOnly: true,
			Description: "The installed version."},
		{Name: "update_available", Param: "updateAvailable", Type: framework.Bool, Key: "updateAvailable", ReadOnly: true,
			Description: "Whether the DNS App Store has a newer version."},
		{Name: "update_version", Param: "updateVersion", Type: framework.String, Key: "updateVersion", ReadOnly: true,
			Description: "The newest version in the DNS App Store."},
	},
}

// appConfig is the config file of an installed app. There is nothing to
// create or delete: the resource writes the file, and destroying it leaves
// the file as it is.
var appConfig = framework.Spec{
	Name:        "app_config",
	Description: "Manages the config of an installed DNS app. The format is the app's own, usually JSON. Destroying the resource leaves the config as it is.",
	Read:        "/api/apps/config/get",
	Update:      "/api/apps/config/set",
	Attrs: []framework.Attr{
		{Name: "name", Param: "name", Type: framework.String, Required: true, Identity: true, Update: true,
			Description: "The name of the installed app."},
		{Name: "config", Param: "config", Type: framework.String, Key: "config", Required: true, Update: true, JSON: true,
			Description: "The app's config as JSON, for example from `jsonencode(...)`. Differences in spacing and key order alone are not changes."},
	},
}

// storeApp is an app in the DNS App Store, which the server fetches and
// lists. It only has a data source: there is nothing on the server to manage.
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
