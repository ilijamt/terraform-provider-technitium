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
)

func TestIntegration_DHCP(t *testing.T) {
	shrink := ReadFixture(t, "dhcp", "shrink.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(*terraform.State) error {
			if _, err := api().Call(context.Background(), "/api/dhcp/scopes/get", url.Values{"name": {"lan"}}); err == nil {
				return fmt.Errorf("scope lan still exists")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config:      ReadFixture(t, "dhcp", "main.tf"),
				ExpectError: regexp.MustCompile(`must match`),
			},
			{
				Config: ReadFixture(t, "dhcp", "tables.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_dhcp_scope.lan", "dns_servers.#", "2"),
					resource.TestCheckResourceAttr("technitium_dhcp_scope.lan", "exclusions.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs("technitium_dhcp_scope.lan", "generic_options.*", map[string]string{"code": "150", "value": "C0:A8:32:05"}),
					resource.TestCheckResourceAttrSet("technitium_dhcp_scope.lan", "lease_time_days"),
					resource.TestCheckResourceAttr("technitium_dhcp_reserved_lease.printer", "ip_address", "192.168.50.100"),
				),
			},
			{
				Config: shrink,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("technitium_dhcp_scope.lan", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("technitium_dhcp_reserved_lease.printer", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.TestCheckResourceAttr("technitium_dhcp_scope.lan", "ending_address", "192.168.50.150"),
			},
			{
				PreConfig: func() {
					call(t, "/api/dhcp/scopes/set", url.Values{"name": {"lan"}, "dnsServers": {"192.168.50.9"}})
				},
				Config:           shrink,
				ConfigPlanChecks: expectUpdate("technitium_dhcp_scope.lan"),
				Check:            resource.TestCheckResourceAttr("technitium_dhcp_scope.lan", "dns_servers.#", "2"),
			},
			{
				ResourceName:      "technitium_dhcp_scope.lan",
				ImportState:       true,
				ImportStateId:     "lan",
				ImportStateVerify: true,
			},
			{
				ResourceName:      "technitium_dhcp_reserved_lease.printer",
				ImportState:       true,
				ImportStateId:     "lan/00-11-22-AA-BB-CC",
				ImportStateVerify: true,
			},
			{
				Config: ReadFixture(t, "dhcp", "cleared.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_dhcp_scope.lan", "exclusions.#", "0"),
					resource.TestCheckResourceAttr("technitium_dhcp_scope.lan", "generic_options.#", "0"),
				),
			},
		},
	})
}

func TestIntegration_DHCPScopeEnabled(t *testing.T) {
	enabled := ReadFixture(t, "dhcp_scope_enabled", "enabled.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "dhcp_scope_enabled", "main.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_dhcp_scope.toggle", "enabled", "false"),
					resource.TestCheckResourceAttr("data.technitium_dhcp_scope.toggle", "enabled", "false"),
				),
			},
			{
				Config:           enabled,
				ConfigPlanChecks: expectUpdate("technitium_dhcp_scope.toggle"),
				Check:            resource.TestCheckResourceAttr("technitium_dhcp_scope.toggle", "enabled", "true"),
			},
			{
				PreConfig:        func() { call(t, "/api/dhcp/scopes/disable", url.Values{"name": {"toggle"}}) },
				Config:           enabled,
				ConfigPlanChecks: expectUpdate("technitium_dhcp_scope.toggle"),
				Check:            resource.TestCheckResourceAttr("technitium_dhcp_scope.toggle", "enabled", "true"),
			},
			{
				ResourceName:      "technitium_dhcp_scope.toggle",
				ImportState:       true,
				ImportStateId:     "toggle",
				ImportStateVerify: true,
			},
		},
	})
}
