package technitium

import "github.com/ilijamt/terraform-provider-technitium/internal/framework"

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
	Description: "Manages a user group and its members. The built-in groups (`Administrators`, `DHCP Administrators`, `DNS Administrators`) already exist: import them to manage their members.",
	Create:      "/api/admin/groups/create",
	Read:        "/api/admin/groups/get",
	Update:      "/api/admin/groups/set",
	Delete:      "/api/admin/groups/delete",
	Attrs: []framework.Attr{
		{Name: "name", Param: "group", Type: framework.String, Key: "name", Required: true, Identity: true, Create: true, Update: true,
			Description: "The group name. Changing it replaces the group."},
		{Name: "description", Param: "description", Type: framework.String, Key: "description", Create: true, Update: true,
			Description: "The group description."},
		{Name: "members", Param: "members", Type: framework.List, Key: "members", Update: true,
			Description: "The usernames of the members, in alphabetical order as the server returns them. An empty list removes all members."},
	},
}

// permissions are the user and group rights on one section of the server.
// There is nothing to create or delete: the resource sets the section's
// lists, and destroying it leaves them as they are.
var permissions = framework.Spec{
	Name: "permissions",
	Description: "Manages who may view, modify and delete in one section of the server. The resource owns the section's full lists: users and groups left out lose their rights, " +
		"except the built-in admin groups, which the server always keeps. List those too, as the server has them, or they show as a change on every plan. " +
		"Destroying the resource leaves the permissions as they are.",
	Read:   "/api/admin/permissions/get",
	Update: "/api/admin/permissions/set",
	Attrs: []framework.Attr{
		{Name: "section", Param: "section", Type: framework.String, Key: "section", Required: true, Identity: true, Update: true,
			Enum:        []string{"Dashboard", "Zones", "Cache", "Allowed", "Blocked", "Apps", "DnsClient", "Settings", "DhcpServer", "Administration", "Logs"},
			Description: "The section: `Dashboard`, `Zones`, `Cache`, `Allowed`, `Blocked`, `Apps`, `DnsClient`, `Settings`, `DhcpServer`, `Administration` or `Logs`."},
		{Name: "user_permissions", Param: "userPermissions", Type: framework.Table, Key: "userPermissions", Update: true,
			Columns:     rights("username", "username", "The username."),
			Description: "The rights of individual users. An empty set removes them."},
		{Name: "group_permissions", Param: "groupPermissions", Type: framework.Table, Key: "groupPermissions", Update: true,
			Columns:     rights("name", "name", "The group name."),
			Description: "The rights of groups, including the built-in admin groups the server keeps."},
	},
}

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
