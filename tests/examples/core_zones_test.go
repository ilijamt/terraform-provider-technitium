//go:build integration

package examples

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestIntegration_ForwarderZone(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("178.168.192.in-addr.arpa"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "forwarder_zone", "main.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone.reverse", "type", "Forwarder"),
					resource.TestCheckResourceAttr("technitium_record_fwd.reverse", "forwarder", "192.168.178.2"),
					func(*terraform.State) error {
						if fwd := records(t, "178.168.192.in-addr.arpa", "178.168.192.in-addr.arpa", "FWD"); len(fwd) != 1 {
							return fmt.Errorf("want 1 FWD record, got %d: %v", len(fwd), fwd)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "technitium_record_fwd.reverse",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegration_ZoneUpdateAndRemovedOutOfBand(t *testing.T) {
	disabled := ReadFixture(t, "zone_update_and_removed_out_of_band", "disabled.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("zone-drift.test"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "zone_update_and_removed_out_of_band", "main.tf"),
				Check:  resource.TestCheckResourceAttr("technitium_zone.test", "disabled", "false"),
			},
			{
				Config:           disabled,
				ConfigPlanChecks: expectUpdate("technitium_zone.test"),
				Check:            resource.TestCheckResourceAttr("technitium_zone.test", "disabled", "true"),
			},
			{
				PreConfig:        func() { call(t, "/api/zones/delete", url.Values{"zone": {"zone-drift.test"}}) },
				Config:           disabled,
				ConfigPlanChecks: expectCreate("technitium_zone.test"),
				Check:            resource.TestCheckResourceAttr("technitium_zone.test", "disabled", "true"),
			},
		},
	})
}

func TestIntegration_ZoneListOptions(t *testing.T) {
	cfg := ReadFixture(t, "zone_list_options", "main.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("acl.test"),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone.acl", "query_access_network_acl.#", "2"),
					resource.TestCheckResourceAttr("technitium_zone.acl", "query_access_network_acl.1", "!192.168.1.1"),
					resource.TestCheckResourceAttr("technitium_zone.acl", "notify_name_servers.#", "2"),
				),
			},
			{
				ResourceName:      "technitium_zone.acl",
				ImportState:       true,
				ImportStateId:     "acl.test",
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					call(t, "/api/zones/options/set", url.Values{"zone": {"acl.test"}, "queryAccessNetworkACL": {"10.1.0.0/16"}})
				},
				Config:           cfg,
				ConfigPlanChecks: expectUpdate("technitium_zone.acl"),
				Check:            resource.TestCheckResourceAttr("technitium_zone.acl", "query_access_network_acl.0", "10.0.0.0/8"),
			},
		},
	})
}

func TestIntegration_ZoneUpdateSecurityPolicies(t *testing.T) {
	cfg := ReadFixture(t, "zone_update_security_policies", "main.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("policy.test"),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone.test", "update_security_policies.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("technitium_zone.test", "update_security_policies.*", map[string]string{
						"domain": "*.policy.test", "allowed_types.#": "1", "allowed_types.0": "TXT",
					}),
				),
			},
			{
				PreConfig: func() {
					call(t, "/api/zones/options/set", url.Values{
						"zone": {"policy.test"}, "updateSecurityPolicies": {"update-key|policy.test|A"},
					})
				},
				Config:           cfg,
				ConfigPlanChecks: expectUpdate("technitium_zone.test"),
				Check:            resource.TestCheckResourceAttr("technitium_zone.test", "update_security_policies.#", "2"),
			},
			{
				ResourceName:      "technitium_zone.test",
				ImportState:       true,
				ImportStateId:     "policy.test",
				ImportStateVerify: true,
			},
			{
				Config: ReadFixture(t, "zone_update_security_policies", "cleared.tf"),
				Check:  resource.TestCheckResourceAttr("technitium_zone.test", "update_security_policies.#", "0"),
			},
		},
	})
}

func TestIntegration_SecondaryZones(t *testing.T) {
	_, ip, primary := ownTestServerWith(t)
	for zone, typ := range map[string]string{"secondary.test": "Primary", "stub.test": "Primary", "secfwd.test": "Forwarder"} {
		callWith(t, primary, "/api/zones/create", url.Values{"zone": {zone}, "type": {typ}, "initializeForwarder": {"false"}})
		callWith(t, primary, "/api/zones/options/set", url.Values{"zone": {zone}, "zoneTransfer": {"Allow"}})
	}
	callWith(t, primary, "/api/zones/records/add", url.Values{"zone": {"secondary.test"}, "domain": {"www.secondary.test"}, "type": {"A"}, "ipAddress": {"10.0.0.1"}})

	vars := config.Variables{"primary": config.StringVariable(ip)}
	// The first transfer runs in the background after create.
	transferred := func(*terraform.State) error {
		for range 60 {
			resp, err := api().Call(context.Background(), "/api/zones/records/get", url.Values{"zone": {"secondary.test"}, "domain": {"www.secondary.test"}})
			if err == nil && len(resp["records"].([]any)) > 0 {
				return nil
			}
			time.Sleep(500 * time.Millisecond)
		}
		return fmt.Errorf("www.secondary.test was not transferred from %s", ip)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("secondary.test", "stub.test", "secfwd.test"),
		Steps: []resource.TestStep{
			{
				Config:          ReadFixture(t, "secondary_zones", "main.tf"),
				ConfigVariables: vars,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone.secondary", "type", "Secondary"),
					resource.TestCheckResourceAttr("technitium_zone.secondary", "primary_zone_transfer_protocol", "Tcp"),
					resource.TestCheckResourceAttr("technitium_zone.stub", "type", "Stub"),
					resource.TestCheckResourceAttr("technitium_zone.secfwd", "type", "SecondaryForwarder"),
					transferred,
				),
			},
			{
				Config:           ReadFixture(t, "secondary_zones", "tls.tf"),
				ConfigVariables:  vars,
				ConfigPlanChecks: expectUpdate("technitium_zone.secondary"),
				Check:            resource.TestCheckResourceAttr("technitium_zone.secondary", "primary_zone_transfer_protocol", "Tls"),
			},
			{
				ResourceName:      "technitium_zone.secondary",
				ConfigVariables:   vars,
				ImportState:       true,
				ImportStateId:     "secondary.test",
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegration_AllowedAndBlockedZones(t *testing.T) {
	listed := func(kind, domain string) bool {
		resp := call(t, "/api/"+kind+"/list", url.Values{"domain": {domain}})
		for _, r := range resp["records"].([]any) {
			if r.(map[string]any)["name"] == domain {
				return true
			}
		}
		return false
	}
	blocked := ReadFixture(t, "allowed_and_blocked_zones", "blocked.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(*terraform.State) error {
			for kind, domain := range map[string]string{"blocked": "tracker.block.test", "allowed": "cdn.ads.block.test"} {
				if listed(kind, domain) {
					return fmt.Errorf("%s is still %s", domain, kind)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config:      ReadFixture(t, "allowed_and_blocked_zones", "main.tf"),
				ExpectError: regexp.MustCompile(`must match`),
			},
			{
				Config: blocked,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_blocked_zone.ads", "id", "ads.block.test"),
					func(*terraform.State) error {
						if !listed("blocked", "ads.block.test") || !listed("allowed", "cdn.ads.block.test") {
							return fmt.Errorf("entries missing on the server")
						}
						return nil
					},
				),
			},
			{
				PreConfig:        func() { call(t, "/api/blocked/delete", url.Values{"domain": {"ads.block.test"}}) },
				Config:           blocked,
				ConfigPlanChecks: expectCreate("technitium_blocked_zone.ads"),
			},
			{
				Config:           ReadFixture(t, "allowed_and_blocked_zones", "tracker.tf"),
				ConfigPlanChecks: expectReplace("technitium_blocked_zone.ads"),
				Check: func(*terraform.State) error {
					if listed("blocked", "ads.block.test") {
						return fmt.Errorf("ads.block.test is still blocked")
					}
					return nil
				},
			},
			{
				ResourceName:      "technitium_blocked_zone.ads",
				ImportState:       true,
				ImportStateId:     "tracker.block.test",
				ImportStateVerify: true,
			},
			{
				ResourceName:      "technitium_allowed_zone.cdn",
				ImportState:       true,
				ImportStateId:     "cdn.ads.block.test",
				ImportStateVerify: true,
			},
		},
	})
}
