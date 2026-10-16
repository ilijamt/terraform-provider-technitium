//go:build integration

package examples

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
)

// Every new user joins Everyone, and the server refuses to take anyone out
// of it.
func TestIntegration_SideEveryoneMembers(t *testing.T) {
	acceptance(t)
	c := api()
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/admin/users/delete", url.Values{"user": {"sideeveryone"}})
	})
	members := func() []string {
		return names(call(t, "/api/admin/groups/get", url.Values{"group": {"Everyone"}})["members"], "")
	}
	call(t, "/api/admin/users/create", url.Values{"user": {"sideeveryone"}, "pass": {"sideeveryone-pass"}})
	require.Contains(t, members(), "sideeveryone")

	_, err := c.Call(ctx, "/api/admin/groups/set", url.Values{"group": {"Everyone"}, "members": {"admin"}})
	require.ErrorContains(t, err, "Access was denied")
	assert.Contains(t, members(), "sideeveryone")
}

// Everyone's members are refused at plan time; the data source still reads
// them.
func TestIntegration_SideEveryoneRefused(t *testing.T) {
	const dir = "side_everyone_refused"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:      ReadFixture(t, dir, "main.tf"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("the Everyone group cannot be managed"),
			},
			{
				Config: ReadFixture(t, dir, "data.tf"),
				Check:  resource.TestCheckTypeSetElemAttr("data.technitium_group_members.everyone", "members.*", "admin"),
			},
		},
	})
}

// The server refuses to delete the token a call is made with, as the
// provider would when destroying the token it uses.
func TestIntegration_SideTokenDeleteOwnSession(t *testing.T) {
	acceptance(t)
	c := api()
	token := call(t, "/api/admin/sessions/createToken", url.Values{"user": {"admin"}, "tokenName": {"side-self"}})["token"].(string)
	partial := url.Values{"partialToken": {token[:16]}}
	t.Cleanup(func() {
		_, _ = c.Call(context.Background(), "/api/admin/sessions/delete", partial)
	})

	self := client.New(host, token, "test", false, nil)
	_, err := self.Call(context.Background(), "/api/admin/sessions/delete", partial)
	require.ErrorContains(t, err, "cannot delete current session")

	call(t, "/api/admin/sessions/delete", partial)
}

// Deleting a user deletes its tokens, rights and group memberships, and
// deleting a group its members and rights; creating them again restores none.
func TestIntegration_SideUserGroupDelete(t *testing.T) {
	acceptance(t)
	c := api()
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/admin/users/delete", url.Values{"user": {"sideuser"}})
		_, _ = c.Call(ctx, "/api/admin/groups/delete", url.Values{"group": {"SideGroup"}})
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-user.test"}})
	})
	call(t, "/api/zones/create", url.Values{"zone": {"side-user.test"}})
	call(t, "/api/admin/users/create", url.Values{"user": {"sideuser"}, "pass": {"sideuser-pass"}})
	call(t, "/api/admin/groups/create", url.Values{"group": {"SideGroup"}})
	call(t, "/api/admin/groups/set", url.Values{"group": {"SideGroup"}, "members": {"sideuser"}})
	call(t, "/api/zones/permissions/set", url.Values{"zone": {"side-user.test"}, "userPermissions": {"sideuser|true|false|false"}, "groupPermissions": {"SideGroup|true|false|false"}})
	call(t, "/api/admin/permissions/set", url.Values{"section": {"Dashboard"}, "userPermissions": {"sideuser|true|false|false"}})
	token := call(t, "/api/admin/sessions/createToken", url.Values{"user": {"sideuser"}, "tokenName": {"side"}})["token"].(string)

	sessions := func() []string {
		var out []string
		for _, s := range call(t, "/api/admin/sessions/list", nil)["sessions"].([]any) {
			if s := s.(map[string]any); s["username"] == "sideuser" {
				out = append(out, s["partialToken"].(string))
			}
		}
		return out
	}
	zone := func() map[string]any {
		return call(t, "/api/zones/permissions/get", url.Values{"zone": {"side-user.test"}})
	}
	members := func() []string {
		return names(call(t, "/api/admin/groups/get", url.Values{"group": {"SideGroup"}})["members"], "")
	}
	dashboard := func() []string {
		return names(call(t, "/api/admin/permissions/get", url.Values{"section": {"Dashboard"}})["userPermissions"], "username")
	}
	require.Equal(t, []string{token[:16]}, sessions())
	require.Equal(t, []string{"sideuser"}, members())
	require.Contains(t, dashboard(), "sideuser")
	require.Contains(t, names(zone()["userPermissions"], "username"), "sideuser")
	require.Contains(t, names(zone()["groupPermissions"], "name"), "SideGroup")

	call(t, "/api/admin/users/delete", url.Values{"user": {"sideuser"}})
	call(t, "/api/admin/users/create", url.Values{"user": {"sideuser"}, "pass": {"sideuser-pass"}})
	assert.Empty(t, sessions(), "tokens")
	assert.Empty(t, members(), "group memberships")
	assert.NotContains(t, dashboard(), "sideuser")
	assert.NotContains(t, names(zone()["userPermissions"], "username"), "sideuser")

	call(t, "/api/admin/groups/set", url.Values{"group": {"SideGroup"}, "members": {"sideuser"}})
	call(t, "/api/admin/groups/delete", url.Values{"group": {"SideGroup"}})
	call(t, "/api/admin/groups/create", url.Values{"group": {"SideGroup"}})
	assert.Empty(t, members(), "group members")
	assert.NotContains(t, names(zone()["groupPermissions"], "name"), "SideGroup")
}

// The examples' replace_triggered_by sets everything again when the user or
// group is replaced.
func TestIntegration_SideUserGroupReplaceTriggered(t *testing.T) {
	config := ReadFixture(t, "side_user_group_replace_triggered", "main.tf")
	intact := func(*terraform.State) error {
		perms := call(t, "/api/zones/permissions/get", url.Values{"zone": {"side-trigger.test"}})
		members := names(call(t, "/api/admin/groups/get", url.Values{"group": {"SideTrigger"}})["members"], "")
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

func TestIntegration_UserPasswordNotResentOnUnrelatedChange(t *testing.T) {
	const dir = "user_password_not_resent_on_unrelated_change"
	acceptance(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, dir, "main.tf")},
			{
				PreConfig: func() {
					call(t, "/api/admin/users/set", url.Values{"user": {"pwuser"}, "newPass": {"oob-secret"}})
				},
				Config: ReadFixture(t, dir, "update.tf"),
				Check: func(*terraform.State) error {
					c := client.New(host, "", "test", false, nil)
					return c.Login(context.Background(), "pwuser", "oob-secret")
				},
			},
		},
	})
}
