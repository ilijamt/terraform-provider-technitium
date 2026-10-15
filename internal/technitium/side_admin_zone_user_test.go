package technitium_test

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deleting a user deletes its tokens, rights and group memberships, and
// deleting a group its members and rights; creating them again restores none.
func TestAccSideUserGroupDelete(t *testing.T) {
	acceptance(t)
	c := api(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/admin/users/delete", url.Values{"user": {"sideuser"}})
		_, _ = c.Call(ctx, "/api/admin/groups/delete", url.Values{"group": {"SideGroup"}})
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-user.test"}})
	})
	call(t, c, "/api/zones/create", url.Values{"zone": {"side-user.test"}})
	setup := func() {
		call(t, c, "/api/admin/users/create", url.Values{"user": {"sideuser"}, "pass": {"sideuser-pass"}})
		call(t, c, "/api/admin/groups/create", url.Values{"group": {"SideGroup"}})
		call(t, c, "/api/admin/groups/set", url.Values{"group": {"SideGroup"}, "members": {"sideuser"}})
		call(t, c, "/api/zones/permissions/set", url.Values{"zone": {"side-user.test"}, "userPermissions": {"sideuser|true|false|false"}, "groupPermissions": {"SideGroup|true|false|false"}})
		call(t, c, "/api/admin/permissions/set", url.Values{"section": {"Dashboard"}, "userPermissions": {"sideuser|true|false|false"}})
	}
	setup()
	token := call(t, c, "/api/admin/sessions/createToken", url.Values{"user": {"sideuser"}, "tokenName": {"side"}})["token"].(string)

	sessions := func() []string {
		var out []string
		for _, s := range call(t, c, "/api/admin/sessions/list", nil)["sessions"].([]any) {
			if s := s.(map[string]any); s["username"] == "sideuser" {
				out = append(out, s["partialToken"].(string))
			}
		}
		return out
	}
	zone := func() map[string]any {
		return call(t, c, "/api/zones/permissions/get", url.Values{"zone": {"side-user.test"}})
	}
	members := func() []string {
		return names(call(t, c, "/api/admin/groups/get", url.Values{"group": {"SideGroup"}})["members"], "")
	}
	dashboard := func() []string {
		return names(call(t, c, "/api/admin/permissions/get", url.Values{"section": {"Dashboard"}})["userPermissions"], "username")
	}
	require.Equal(t, []string{token[:16]}, sessions())
	require.Equal(t, []string{"sideuser"}, members())
	require.Contains(t, dashboard(), "sideuser")
	require.Contains(t, names(zone()["userPermissions"], "username"), "sideuser")
	require.Contains(t, names(zone()["groupPermissions"], "name"), "SideGroup")

	call(t, c, "/api/admin/users/delete", url.Values{"user": {"sideuser"}})
	call(t, c, "/api/admin/users/create", url.Values{"user": {"sideuser"}, "pass": {"sideuser-pass"}})
	assert.Empty(t, sessions(), "tokens")
	assert.Empty(t, members(), "group memberships")
	assert.NotContains(t, dashboard(), "sideuser")
	assert.NotContains(t, names(zone()["userPermissions"], "username"), "sideuser")

	call(t, c, "/api/admin/groups/set", url.Values{"group": {"SideGroup"}, "members": {"sideuser"}})
	call(t, c, "/api/admin/groups/delete", url.Values{"group": {"SideGroup"}})
	call(t, c, "/api/admin/groups/create", url.Values{"group": {"SideGroup"}})
	assert.Empty(t, members(), "group members")
	assert.NotContains(t, names(zone()["groupPermissions"], "name"), "SideGroup")
}

// The examples' replace_triggered_by sets everything again when the user or
// group is replaced.
func TestAccSideUserGroupReplaceTriggered(t *testing.T) {
	config := `
resource "technitium_zone" "z" {
  zone = "side-trigger.test"
}

resource "technitium_user" "u" {
  username = "sidetrigger"
  password = "sidetrigger-pass"
}

resource "technitium_group" "g" {
  name = "SideTrigger"
}

resource "technitium_group_members" "g" {
  group   = technitium_group.g.name
  members = [technitium_user.u.username]

  lifecycle {
    replace_triggered_by = [technitium_user.u, technitium_group.g]
  }
}

resource "technitium_zone_permissions" "z" {
  zone = technitium_zone.z.zone
  user_permissions = [
    { username = technitium_user.u.username, can_view = true, can_modify = false, can_delete = false },
  ]
  group_permissions = [
    { name = technitium_group.g.name, can_view = true, can_modify = false, can_delete = false },
  ]

  lifecycle {
    replace_triggered_by = [technitium_zone.z, technitium_user.u, technitium_group.g]
  }
}

resource "technitium_api_token" "t" {
  username   = technitium_user.u.username
  token_name = "side"

  lifecycle {
    replace_triggered_by = [technitium_user.u]
  }
}
`
	intact := func(*terraform.State) error {
		c := api(t)
		perms := call(t, c, "/api/zones/permissions/get", url.Values{"zone": {"side-trigger.test"}})
		members := names(call(t, c, "/api/admin/groups/get", url.Values{"group": {"SideTrigger"}})["members"], "")
		if fmt.Sprint(members) != "[sidetrigger]" {
			return fmt.Errorf("members %v", members)
		}
		if !assert.Contains(t, names(perms["userPermissions"], "username"), "sidetrigger") ||
			!assert.Contains(t, names(perms["groupPermissions"], "name"), "SideTrigger") {
			return fmt.Errorf("zone permissions lost: %v", perms)
		}
		return nil
	}
	replaced := func(addrs ...string) resource.ConfigPlanChecks {
		var checks []plancheck.PlanCheck
		for _, a := range addrs {
			checks = append(checks, plancheck.ExpectResourceAction(a, plancheck.ResourceActionDestroyBeforeCreate))
		}
		return resource.ConfigPlanChecks{PreApply: checks}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("side-trigger.test"),
		Steps: []resource.TestStep{
			{Config: config, Check: intact},
			{
				Config: config, Taint: []string{"technitium_user.u"},
				ConfigPlanChecks: replaced("technitium_user.u", "technitium_group_members.g", "technitium_zone_permissions.z", "technitium_api_token.t"),
				Check:            intact,
			},
			{
				Config: config, Taint: []string{"technitium_group.g"},
				ConfigPlanChecks: replaced("technitium_group.g", "technitium_group_members.g", "technitium_zone_permissions.z"),
				Check:            intact,
			},
			{Config: config, ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
		},
	})
}
