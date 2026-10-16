//go:build integration

package examples

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
)

// coreRestoreZonesPermissions puts the rights in the Zones section back when
// the test ends. Destroying technitium_permissions_zones leaves them as set.
func coreRestoreZonesPermissions(t *testing.T) {
	resp := call(t, "/api/admin/permissions/get", url.Values{"section": {"Zones"}})
	rows := func(key, name string) string {
		var cells []string
		list, _ := resp[key].([]any)
		for _, r := range list {
			p := r.(map[string]any)
			cells = append(cells, fmt.Sprint(p[name]), fmt.Sprint(p["canView"]), fmt.Sprint(p["canModify"]), fmt.Sprint(p["canDelete"]))
		}
		return strings.Join(cells, "|")
	}
	params := url.Values{
		"section":          {"Zones"},
		"userPermissions":  {rows("userPermissions", "username")},
		"groupPermissions": {rows("groupPermissions", "name")},
	}
	t.Cleanup(func() {
		_, err := api().Call(context.Background(), "/api/admin/permissions/set", params)
		if err != nil {
			t.Errorf("restore the Zones permissions: %s", err)
		}
	})
}

func TestIntegration_UsersGroupsAndPermissions(t *testing.T) {
	acceptance(t)
	coreRestoreZonesPermissions(t)
	password := ReadFixture(t, "users_groups_and_permissions", "password.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(*terraform.State) error {
			c := api()
			if _, err := c.Call(context.Background(), "/api/admin/users/get", url.Values{"user": {"alice"}}); err == nil {
				return fmt.Errorf("user alice still exists")
			}
			if _, err := c.Call(context.Background(), "/api/admin/groups/get", url.Values{"group": {"Ops"}}); err == nil {
				return fmt.Errorf("group Ops still exists")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config:      ReadFixture(t, "users_groups_and_permissions", "main.tf"),
				ExpectError: regexp.MustCompile(`must match`),
			},
			{
				Config: ReadFixture(t, "users_groups_and_permissions", "lower_case.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_group_members.ops", "members.0", "alice"),
					resource.TestCheckResourceAttr("technitium_permissions_zones.this", "group_permissions.#", "1"),
					resource.TestCheckResourceAttr("technitium_user.alice", "session_timeout_seconds", "1800"),
				),
			},
			{
				Config:           password,
				ConfigPlanChecks: expectUpdate("technitium_user.alice"),
				Check: func(*terraform.State) error {
					return client.New(host, "", "test", false, nil).Login(context.Background(), "alice", "second-secret")
				},
			},
			{
				PreConfig:        func() { call(t, "/api/admin/groups/set", url.Values{"group": {"Ops"}, "members": {""}}) },
				Config:           password,
				ConfigPlanChecks: expectUpdate("technitium_group_members.ops"),
				Check:            resource.TestCheckResourceAttr("technitium_group_members.ops", "members.#", "1"),
			},
			{
				ResourceName:            "technitium_user.alice",
				ImportState:             true,
				ImportStateId:           "alice",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			{
				ResourceName:      "technitium_group.ops",
				ImportState:       true,
				ImportStateId:     "Ops",
				ImportStateVerify: true,
			},
			{
				ResourceName:      "technitium_group_members.ops",
				ImportState:       true,
				ImportStateId:     "Ops",
				ImportStateVerify: true,
			},
			{
				ResourceName:      "technitium_permissions_zones.this",
				ImportState:       true,
				ImportStateId:     "permissions_zones",
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegration_DataSources(t *testing.T) {
	acceptance(t)
	coreRestoreZonesPermissions(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("data.test"),
		Steps: []resource.TestStep{
			{
				Config:      ReadFixture(t, "data_sources", "main.tf"),
				ExpectError: regexp.MustCompile(`not found`),
			},
			{
				Config: ReadFixture(t, "data_sources", "found.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.technitium_zone.test", "type", "Primary"),
					resource.TestCheckResourceAttr("data.technitium_group.admins", "description", "Super administrators"),
					resource.TestCheckResourceAttr("data.technitium_user.admin", "display_name", "Administrator"),
					resource.TestCheckNoResourceAttr("data.technitium_user.admin", "password"),
					resource.TestCheckResourceAttr("data.technitium_user.admin", "is_sso_user", "false"),
					resource.TestCheckResourceAttrSet("data.technitium_settings_general.this", "dns_server_domain"),
					resource.TestCheckResourceAttr("data.technitium_dhcp_scope.test", "starting_address", "192.168.80.10"),
					resource.TestCheckTypeSetElemNestedAttrs("data.technitium_permissions_zones.this", "group_permissions.*", map[string]string{"name": "DataOps"}),
					resource.TestCheckResourceAttr("data.technitium_permissions_zones.this", "group_permissions.#", "1"),
					resource.TestCheckResourceAttr("technitium_permissions_zones.this", "group_permissions.#", "1"),
				),
			},
		},
	})
}

func TestIntegration_KeptGroupNeedsFullRights(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      ReadFixture(t, "kept_group_needs_full_rights", "main.tf"),
			ExpectError: regexp.MustCompile(`always has full rights`),
		}},
	})
}

func TestIntegration_ZonePermissions(t *testing.T) {
	cfg := ReadFixture(t, "zone_permissions", "main.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("perm.test"),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone_permissions.test", "group_permissions.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs("data.technitium_zone_permissions.test", "user_permissions.*", map[string]string{"username": "bob", "can_modify": "true"}),
				),
			},
			{
				PreConfig: func() {
					call(t, "/api/zones/permissions/set", url.Values{"zone": {"perm.test"}, "userPermissions": {""}})
				},
				Config:           cfg,
				ConfigPlanChecks: expectUpdate("technitium_zone_permissions.test"),
				Check:            resource.TestCheckResourceAttr("technitium_zone_permissions.test", "user_permissions.#", "1"),
			},
			{
				ResourceName:      "technitium_zone_permissions.test",
				ImportState:       true,
				ImportStateId:     "perm.test",
				ImportStateVerify: true,
			},
			{
				Config:           ReadFixture(t, "zone_permissions", "builtin.tf"),
				ConfigPlanChecks: expectUpdate("technitium_zone_permissions.test"),
				Check:            resource.TestCheckResourceAttr("technitium_zone_permissions.test", "group_permissions.#", "3"),
			},
			{
				Config: cfg,
				Check:  resource.TestCheckResourceAttr("technitium_zone_permissions.test", "group_permissions.#", "1"),
			},
		},
	})
}

func TestIntegration_APIToken(t *testing.T) {
	var apiToken string
	works := func(token string) error {
		_, err := client.New(host, token, "test", false, nil).Call(context.Background(), "/api/user/session/get", nil)
		return err
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(*terraform.State) error {
			if works(apiToken) == nil {
				return fmt.Errorf("the token still works after destroy")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "api_token", "main.tf"),
				Check: func(s *terraform.State) error {
					attrs := s.RootModule().Resources["technitium_api_token.ci"].Primary.Attributes
					apiToken = attrs["token"]
					if len(apiToken) < 16 || attrs["partial_token"] != apiToken[:16] || attrs["id"] != apiToken[:16] {
						return fmt.Errorf("partial token %q does not start the token", attrs["partial_token"])
					}
					return works(apiToken)
				},
			},
			{
				ResourceName:            "technitium_api_token.ci",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"token"},
			},
		},
	})
}
