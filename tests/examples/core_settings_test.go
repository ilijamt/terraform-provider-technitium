//go:build integration

package examples

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/stretchr/testify/assert"
)

// coreSettingsColumns orders the cells of the settings tables the way
// /api/settings/set takes them.
var coreSettingsColumns = map[string][]string{
	"tsigKeys":            {"keyName", "sharedSecret", "algorithmName"},
	"qpmPrefixLimitsIPv4": {"prefix", "udpLimit", "tcpLimit"},
}

// coreRestoreSettings puts the server settings named by keys back when the
// test ends. Destroying a settings resource leaves the settings as set.
func coreRestoreSettings(t *testing.T, keys ...string) {
	resp := call(t, "/api/settings/get", nil)
	params := url.Values{}
	for _, k := range keys {
		var cells []string
		switch v := resp[k].(type) {
		case nil:
		case []any:
			for _, e := range v {
				row, ok := e.(map[string]any)
				if !ok {
					cells = append(cells, fmt.Sprint(e))
					continue
				}
				for _, c := range coreSettingsColumns[k] {
					cells = append(cells, fmt.Sprint(row[c]))
				}
			}
		default:
			cells = append(cells, fmt.Sprint(v))
		}
		sep := ","
		if coreSettingsColumns[k] != nil {
			sep = "|"
		}
		params.Set(k, strings.Join(cells, sep))
	}
	t.Cleanup(func() {
		if _, err := api().Call(context.Background(), "/api/settings/set", params); err != nil {
			t.Errorf("restore settings %v: %s", keys, err)
		}
	})
}

func TestIntegration_SettingsSSO(t *testing.T) {
	acceptance(t)
	resetSSO(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "settings_sso", "main.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_settings_sso.this", "client_id", "technitium"),
					resource.TestCheckResourceAttr("technitium_settings_sso.this", "scopes.#", "3"),
					resource.TestCheckResourceAttr("data.technitium_settings_sso.this", "authority", "https://idp.example.com/realms/home"),
					resource.TestCheckNoResourceAttr("data.technitium_settings_sso.this", "client_secret"),
					resource.TestCheckTypeSetElemNestedAttrs("data.technitium_settings_sso.this", "group_map.*", map[string]string{"remote_group": "dns-admins", "local_group": "DNS Administrators"}),
				),
			},
			{
				Config:           ReadFixture(t, "settings_sso", "update.tf"),
				ConfigPlanChecks: expectUpdate("technitium_settings_sso.this"),
				Check:            resource.TestCheckTypeSetElemNestedAttrs("technitium_settings_sso.this", "group_map.*", map[string]string{"remote_group": "dns-operators"}),
			},
			{
				ResourceName:            "technitium_settings_sso.this",
				ImportState:             true,
				ImportStateId:           "settings_sso",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"client_secret"},
			},
		},
	})
}

func TestIntegration_SettingsLDAP(t *testing.T) {
	acceptance(t)
	resetLDAP(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "settings_ldap", "main.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_settings_ldap.this", "port", "636"),
					resource.TestCheckResourceAttr("data.technitium_settings_ldap.this", "ssl_option", "LDAPS"),
					resource.TestCheckNoResourceAttr("data.technitium_settings_ldap.this", "bind_password"),
					resource.TestCheckTypeSetElemNestedAttrs("data.technitium_settings_ldap.this", "group_map.*", map[string]string{"local_group": "DNS Administrators"}),
				),
			},
			{
				Config:           ReadFixture(t, "settings_ldap", "update.tf"),
				ConfigPlanChecks: expectUpdate("technitium_settings_ldap.this"),
				Check:            resource.TestCheckTypeSetElemNestedAttrs("technitium_settings_ldap.this", "group_map.*", map[string]string{"remote_group": "CN=dns-ops,DC=example,DC=com"}),
			},
			{
				ResourceName:            "technitium_settings_ldap.this",
				ImportState:             true,
				ImportStateId:           "settings_ldap",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"bind_password"},
			},
		},
	})
}

func TestIntegration_Settings(t *testing.T) {
	acceptance(t)
	coreRestoreSettings(t, "dnsServerDomain", "forwarders", "recursion")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "settings", "main.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_settings_general.this", "id", "settings_general"),
					resource.TestCheckResourceAttr("technitium_settings_general.this", "dns_server_domain", "ns.settings.test"),
					resource.TestCheckResourceAttr("technitium_settings_proxy_forwarders.this", "forwarders.#", "2"),
					resource.TestCheckResourceAttr("technitium_settings_recursion.this", "recursion", "Allow"),
				),
			},
			{
				Config: ReadFixture(t, "settings", "update.tf"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("technitium_settings_proxy_forwarders.this", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("technitium_settings_general.this", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("technitium_settings_recursion.this", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_settings_proxy_forwarders.this", "forwarders.#", "0"),
					resource.TestCheckResourceAttr("technitium_settings_general.this", "dns_server_domain", "ns.settings.test"),
				),
			},
			{
				ResourceName:      "technitium_settings_proxy_forwarders.this",
				ImportState:       true,
				ImportStateId:     "settings_proxy_forwarders",
				ImportStateVerify: true,
			},
		},
	})

	resp := call(t, "/api/settings/get", nil)
	assert.Equal(t, "Allow", resp["recursion"], "destroy leaves the settings alone")
	assert.Equal(t, "ns.settings.test", resp["dnsServerDomain"])
}

func TestIntegration_SettingsTables(t *testing.T) {
	acceptance(t)
	coreRestoreSettings(t, "tsigKeys", "qpmPrefixLimitsIPv4")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "settings_tables", "main.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_settings_tsig.this", "tsig_keys.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("technitium_settings_tsig.this", "tsig_keys.*", map[string]string{"key_name": "alpha", "algorithm_name": "hmac-sha512"}),
					resource.TestCheckResourceAttr("technitium_settings_general.this", "qpm_prefix_limits_ipv4.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs("technitium_settings_general.this", "qpm_prefix_limits_ipv4.*", map[string]string{"prefix": "24", "udp_limit": "6000", "tcp_limit": "0"}),
				),
			},
			{
				Config:           ReadFixture(t, "settings_tables", "update.tf"),
				ConfigPlanChecks: expectUpdate("technitium_settings_tsig.this"),
				Check:            resource.TestCheckResourceAttr("technitium_settings_tsig.this", "tsig_keys.#", "1"),
			},
			{
				ResourceName:      "technitium_settings_tsig.this",
				ImportState:       true,
				ImportStateId:     "settings_tsig",
				ImportStateVerify: true,
			},
			{
				Config: ReadFixture(t, "settings_tables", "cleared.tf"),
				Check:  resource.TestCheckResourceAttr("technitium_settings_tsig.this", "tsig_keys.#", "0"),
			},
		},
	})

	resp := call(t, "/api/settings/get", nil)
	assert.Empty(t, resp["tsigKeys"])
}
