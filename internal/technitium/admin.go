package technitium

import (
	"fmt"

	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
)

// username is how the server stores usernames: in lower case.
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
		{Name: "password", Param: "newPass", CreateParam: "pass", Type: framework.String, Required: true, Sensitive: true, Create: true, Update: true,
			Description: "The password. The server never returns it, so a password changed outside Terraform is not detected; changing it here resets it."},
		{Name: "display_name", Param: "displayName", Type: framework.String, Key: "displayName", Create: true, Update: true,
			Description: "The display name."},
		{Name: "disabled", Param: "disabled", Type: framework.Bool, Key: "disabled", Update: true,
			Description: "Disable the account. Disabling it also ends its active sessions."},
		{Name: "session_timeout_seconds", Param: "sessionTimeoutSeconds", Type: framework.Int64, Key: "sessionTimeoutSeconds", Update: true,
			Description: "The seconds of inactivity after which a session expires. Technitium's default is `1800`."},
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

// groupMembers is the member list of a group, apart from the group so the
// built-in groups' members can be managed too. The server only takes the
// whole list, so the resource owns it; destroying it leaves the members.
var groupMembers = framework.Spec{
	Name: "group_members",
	Description: "Manages the full member list of a group, built-in groups (`Administrators`, `DHCP Administrators`, `DNS Administrators`) included. " +
		"Users left out are removed from the group. Destroying the resource leaves the members as they are.",
	Read:   "/api/admin/groups/get",
	Update: "/api/admin/groups/set",
	Attrs: []framework.Attr{
		{Name: "group", Param: "group", Type: framework.String, Key: "name", Required: true, Identity: true, Update: true,
			Description: "The group name. Changing it moves the resource to another group."},
		{Name: "members", Param: "members", Type: framework.List, Key: "members", Required: true, Update: true,
			Description: "The usernames of the members, in alphabetical order as the server returns them. An empty list removes all members."},
	},
}

// permissionSections are the sections of the server, by the name of their
// permissions_<name> resource.
var permissionSections = []struct{ name, section string }{
	{"dashboard", "Dashboard"},
	{"zones", "Zones"},
	{"cache", "Cache"},
	{"allowed", "Allowed"},
	{"blocked", "Blocked"},
	{"apps", "Apps"},
	{"dns_client", "DnsClient"},
	{"settings", "Settings"},
	{"dhcp_server", "DhcpServer"},
	{"administration", "Administration"},
	{"logs", "Logs"},
}

// permissions are the user and group rights on each section of the server.
// There is nothing to create or delete: a resource sets its section's lists,
// and destroying it leaves them as they are.
var permissions = func() []framework.Spec {
	var out []framework.Spec
	for _, s := range permissionSections {
		out = append(out, framework.Spec{
			Name: "permissions_" + s.name,
			Description: fmt.Sprintf("Manages who may view, modify and delete in the %s section. The lists set here are the section's full lists: users and groups left out lose their rights, ", s.section) +
				"except the built-in admin groups, which the server always keeps; list those too, or take them from the data source of the same name. " +
				"Destroying the resource leaves the permissions as they are.",
			Read:   "/api/admin/permissions/get",
			Update: "/api/admin/permissions/set",
			Fixed:  map[string]string{"section": s.section},
			Attrs: []framework.Attr{
				{Name: "user_permissions", Param: "userPermissions", Type: framework.Table, Key: "userPermissions", Update: true,
					Columns:     rights("username", "username", "The username."),
					Description: "The rights of individual users. An empty set removes them; leaving it out keeps the server's."},
				{Name: "group_permissions", Param: "groupPermissions", Type: framework.Table, Key: "groupPermissions", Update: true,
					Columns:     rights("name", "name", "The group name."),
					Description: "The rights of groups, including the built-in admin groups the server keeps. Leaving it out keeps the server's."},
			},
		})
	}
	return out
}()

// rights are the columns of a permission row: who, then view, modify and
// delete, in the order the set call takes them.
func rights(name, key, description string) []framework.Column {
	return []framework.Column{
		{Name: name, Key: key, Type: framework.String, Description: description},
		{Name: "can_view", Key: "canView", Type: framework.Bool, Description: "May view the section."},
		{Name: "can_modify", Key: "canModify", Type: framework.Bool, Description: "May change the section."},
		{Name: "can_delete", Key: "canDelete", Type: framework.Bool, Description: "May delete in the section."},
	}
}
