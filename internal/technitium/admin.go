package technitium

import (
	"fmt"
	"strings"

	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
)

// The server stores usernames in lower case.
const username = `^[^A-Z]+$`

var user = framework.Spec{
	Name:        "user",
	Description: "Manages a user account. Group membership is managed on `technitium_group`.",
	Create:      "/api/admin/users/create",
	Read:        "/api/admin/users/get",
	Update:      "/api/admin/users/set",
	Delete:      "/api/admin/users/delete",
	Attrs: []framework.Attr{
		{Name: "username", Param: "user", Type: framework.String, Key: "username", Required: true, Identity: true, Create: true, Update: true, Pattern: username,
			Description: "The username, in lower case. Changing it replaces the user."},
		{Name: "password", Param: "newPass", CreateParam: "pass", Type: framework.String, Required: true, Sensitive: true, Create: true, Update: true, OnChange: true,
			Description: "The password. The server never returns it, so a password changed outside Terraform is not detected, and is kept until this one changes."},
		{Name: "display_name", Param: "displayName", Type: framework.String, Key: "displayName", Create: true, Update: true,
			Description: "The display name."},
		{Name: "disabled", Param: "disabled", Type: framework.Bool, Key: "disabled", Update: true,
			Description: "Disable the account. Disabling it also ends its active sessions."},
		{Name: "session_timeout_seconds", Param: "sessionTimeoutSeconds", Type: framework.Int64, Key: "sessionTimeoutSeconds", Update: true,
			Description: "The seconds of inactivity after which a session expires. Technitium's default is `1800`."},
		{Name: "is_sso_user", Param: "isSsoUser", Type: framework.Bool, Key: "isSsoUser", ReadOnly: true, DataOnly: true,
			Description: "Whether the account comes from single sign-on. Its password, name and groups are then managed by the SSO provider."},
	},
}

var group = framework.Spec{
	Name:        "group",
	Description: "Manages a user group. Its members are a `technitium_group_members` resource. Not for the built-in groups (`Administrators`, `DHCP Administrators`, `DNS Administrators`, `Everyone`), which exist on every server and cannot be deleted.",
	Create:      "/api/admin/groups/create",
	Read:        "/api/admin/groups/get",
	Update:      "/api/admin/groups/set",
	Delete:      "/api/admin/groups/delete",
	Attrs: []framework.Attr{
		{Name: "name", Param: "group", Type: framework.String, Key: "name", Required: true, Identity: true, Create: true, Update: true,
			Description: "The group name. Changing it replaces the group."},
		{Name: "description", Param: "description", Type: framework.String, Key: "description", Create: true, Update: true,
			Description: "The group description."},
	},
}

// groupMembers is apart from the group so the built-in groups' members can
// be managed too. The server only takes the whole list.
var groupMembers = framework.Spec{
	Name: "group_members",
	Description: "Manages the full member list of a group, built-in groups (`Administrators`, `DHCP Administrators`, `DNS Administrators`) included. " +
		"Users left out are removed from the group. The server adds the user the provider logs in as back to `Administrators`, so list that user there, or apply fails with an inconsistent result. Destroying the resource leaves the members as they are.",
	Read:   "/api/admin/groups/get",
	Update: "/api/admin/groups/set",
	Attrs: []framework.Attr{
		{Name: "group", Param: "group", Type: framework.String, Key: "name", Required: true, Identity: true, Update: true,
			Description: "The group name. Changing it moves the resource to another group."},
		{Name: "members", Param: "members", Type: framework.List, Key: "members", Required: true, Update: true,
			Description: "The usernames of the members, in alphabetical order as the server returns them. An empty list removes all members."},
	},
}

var permissionSections = []struct {
	name, section string
	kept          []string
}{
	{"dashboard", "Dashboard", []string{"Administrators"}},
	{"zones", "Zones", []string{"Administrators", "DNS Administrators"}},
	{"cache", "Cache", []string{"Administrators"}},
	{"allowed", "Allowed", []string{"Administrators"}},
	{"blocked", "Blocked", []string{"Administrators"}},
	{"apps", "Apps", []string{"Administrators"}},
	{"dns_client", "DnsClient", []string{"Administrators"}},
	{"settings", "Settings", []string{"Administrators"}},
	{"dhcp_server", "DhcpServer", []string{"Administrators", "DHCP Administrators"}},
	{"administration", "Administration", []string{"Administrators"}},
	{"logs", "Logs", []string{"Administrators"}},
}

func kept(groups []string) string {
	names := make([]string, len(groups))
	for i, g := range groups {
		names[i] = "`" + g + "`"
	}
	return fmt.Sprintf("The server always keeps %s with full rights, whatever is set: leave them out, or list them with full rights. ", strings.Join(names, " and "))
}

var permissions = func() []framework.Spec {
	var out []framework.Spec
	for _, s := range permissionSections {
		out = append(out, framework.Spec{
			Name: "permissions_" + s.name,
			Description: fmt.Sprintf("Manages who may view, modify and delete in the %s section. The lists set here are the section's full lists: users and groups left out lose their rights. ", s.section) +
				kept(s.kept) + "Destroying the resource leaves the permissions as they are.",
			Read:   "/api/admin/permissions/get",
			Update: "/api/admin/permissions/set",
			Fixed:  map[string]string{"section": s.section},
			Attrs: []framework.Attr{
				{Name: "user_permissions", Param: "userPermissions", Type: framework.Table, Key: "userPermissions", Update: true,
					Columns:     rights("username", "username", "The username."),
					Description: "The rights of individual users. An empty set removes them; leaving it out keeps the server's."},
				{Name: "group_permissions", Param: "groupPermissions", Type: framework.Table, Key: "groupPermissions", Update: true,
					Columns: rights("name", "name", "The group name."), Kept: s.kept,
					Description: "The rights of groups. " + kept(s.kept) + "Leaving it out keeps the server's."},
			},
		})
	}
	return out
}()

// The token is only in the create call's response; the session list
// identifies it by its first 16 characters.
var apiToken = framework.Spec{
	Name: "api_token",
	Description: "Issues an API token for a user, for tools that call the HTTP API. The token is only known when it is created, so an imported token has no `token` value. " +
		"Changing the user or name issues a new token; destroying the resource revokes it.",
	Create: "/api/admin/sessions/createToken",
	Read:   "/api/admin/sessions/list",
	Delete: "/api/admin/sessions/delete",
	List:   "sessions",
	Created: func(resp map[string]any) map[string]any {
		token, _ := resp["token"].(string)
		return map[string]any{"token": token, "partial_token": token[:min(16, len(token))]}
	},
	Attrs: []framework.Attr{
		{Name: "username", Param: "user", Type: framework.String, Key: "username", Required: true, Create: true, Pattern: username,
			Description: "The user the token acts as, in lower case."},
		{Name: "token_name", Param: "tokenName", Type: framework.String, Key: "tokenName", Required: true, Create: true,
			Description: "A name for the token. Names need not be unique."},
		{Name: "partial_token", Param: "partialToken", Type: framework.String, Key: "partialToken", Identity: true, ReadOnly: true,
			Description: "The first 16 characters of the token, which the server shows to identify it. It is the import ID."},
		{Name: "token", Param: "token", Type: framework.String, Sensitive: true, ReadOnly: true,
			Description: "The token, for the `Authorization: Bearer` header or the provider's `token`."},
	},
}

var zoneKept = []string{"Administrators", "DNS Administrators"}

var zonePermissions = framework.Spec{
	Name: "zone_permissions",
	Description: "Manages who may view, modify and delete one zone, on top of `technitium_permissions_zones`. The lists set here are the zone's full lists: users and groups left out lose their rights to it. " +
		kept(zoneKept) + "Destroying the resource leaves the permissions as they are.",
	Read:   "/api/zones/permissions/get",
	Update: "/api/zones/permissions/set",
	Attrs: []framework.Attr{
		{Name: "zone", Param: "zone", Type: framework.String, Key: "subItem", Required: true, Identity: true, Update: true,
			Description: "The zone. Changing it moves the resource to another zone."},
		{Name: "user_permissions", Param: "userPermissions", Type: framework.Table, Key: "userPermissions", Update: true,
			Columns:     rights("username", "username", "The username."),
			Description: "The rights of individual users on the zone. An empty set removes them; leaving it out keeps the server's."},
		{Name: "group_permissions", Param: "groupPermissions", Type: framework.Table, Key: "groupPermissions", Update: true,
			Columns: rights("name", "name", "The group name."), Kept: zoneKept,
			Description: "The rights of groups on the zone. " + kept(zoneKept) + "Leaving it out keeps the server's."},
	},
}

func rights(name, key, description string) []framework.Column {
	return []framework.Column{
		{Name: name, Key: key, Type: framework.String, Description: description},
		{Name: "can_view", Key: "canView", Type: framework.Bool, Description: "May view the section."},
		{Name: "can_modify", Key: "canModify", Type: framework.Bool, Description: "May change the section."},
		{Name: "can_delete", Key: "canDelete", Type: framework.Bool, Description: "May delete in the section."},
	}
}
