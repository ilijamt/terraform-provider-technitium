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
	"github.com/stretchr/testify/require"
)

func TestIntegration_RecordValuesKeepCase(t *testing.T) {
	config := ReadFixture(t, "record_values_keep_case", "main.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("case.test"),
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig: func() {
					base := url.Values{"zone": {"case.test"}, "domain": {"case.test"}, "type": {"TXT"}}
					call(t, "/api/zones/records/update", merge(base, url.Values{"text": {"hello"}, "newText": {"HELLO"}}))
				},
				Config:           config,
				ConfigPlanChecks: expectCreate("technitium_record_txt.hello"),
			},
			{
				PreConfig: func() {
					base := url.Values{"zone": {"case.test"}, "domain": {"case.test"}, "type": {"CAA"}, "flags": {"0"}, "tag": {"issue"}}
					call(t, "/api/zones/records/update", merge(base, url.Values{"value": {"ca.example"}, "newValue": {"CA.example"}}))
				},
				Config:           config,
				ConfigPlanChecks: expectCreate("technitium_record_caa.issue"),
			},
		},
	})
}

// TestIntegration_NamesWithTrailingDot: the server stores names without the
// trailing dot, which must not fail the apply.
func TestIntegration_NamesWithTrailingDot(t *testing.T) {
	config := ReadFixture(t, "names_with_trailing_dot", "main.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("dot.test"),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_cname.www", "cname", "Target.Example.NET."),
					resource.TestCheckResourceAttr("technitium_record_soa.soa", "primary_name_server", "ns1.dot.test."),
				),
			},
			{
				PreConfig: func() {
					call(t, "/api/zones/records/update", url.Values{"zone": {"dot.test"}, "domain": {"www.dot.test"}, "type": {"CNAME"}, "cname": {"other.example.net"}})
				},
				Config:           config,
				ConfigPlanChecks: expectUpdate("technitium_record_cname.www"),
			},
		},
	})
}

// TestIntegration_AddressSpelling: an IPv6 address the server writes in
// another spelling still matches, and request-ip-address is refused.
func TestIntegration_AddressSpelling(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("ipv6.test"),
		Steps: []resource.TestStep{
			{
				Config:      ReadFixture(t, "address_spelling", "request_ip.tf"),
				ExpectError: regexp.MustCompile(`must be an IP address`),
			},
			{Config: ReadFixture(t, "address_spelling", "main.tf")},
			{
				ResourceName:      "technitium_record_aaaa.www",
				ImportState:       true,
				ImportStateId:     "ipv6.test/www.ipv6.test/2001:DB8:0::1",
				ImportStateVerify: true,
			},
		},
	})
}

func TestIntegration_URIRecordNormalized(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("uri.test"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "uri_record_normalized", "main.tf"),
				Check:  resource.TestCheckResourceAttr("technitium_record_uri.web", "uri", "HTTPS://Example.com"),
			},
			{
				ResourceName:      "technitium_record_uri.web",
				ImportState:       true,
				ImportStateId:     "uri.test/_http._tcp.uri.test/10/1/HTTPS://Example.com",
				ImportStateVerify: true,
			},
		},
	})
}

// TestIntegration_IDN: the server returns internationalized names as punycode.
func TestIntegration_IDN(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("xn--bcher-kva.test"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "idn", "main.tf"),
				Check:  resource.TestCheckResourceAttr("technitium_record_a.www", "domain", "www.bücher.test"),
			},
			{
				ResourceName:      "technitium_record_a.www",
				ImportState:       true,
				ImportStateId:     "bücher.test/www.bücher.test/10.0.0.1",
				ImportStateVerify: true,
			},
		},
	})
}

// TestIntegration_ServiceBindingAutoHints: the server adds the hints to svcParams.
func TestIntegration_ServiceBindingAutoHints(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("hints.test"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, "service_binding_auto_hints", "main.tf"),
				Check: func(*terraform.State) error {
					recs := records(t, "hints.test", "www.hints.test", "HTTPS")
					if len(recs) != 1 {
						return fmt.Errorf("%d HTTPS records", len(recs))
					}
					if _, ok := recs[0]["rData"].(map[string]any)["svcParams"].(map[string]any)["ipv4hint"]; !ok {
						return fmt.Errorf("no ipv4hint: %v", recs[0]["rData"])
					}
					return nil
				},
			},
			{Config: ReadFixture(t, "service_binding_auto_hints", "update.tf"), ConfigPlanChecks: expectUpdate("technitium_record_https.www")},
			{
				ResourceName:      "technitium_record_https.www",
				ImportState:       true,
				ImportStateId:     `{"zone": "hints.test", "domain": "www.hints.test", "svc_priority": 1, "svc_target_name": "www.hints.test", "svc_params": {"alpn": "h2"}}`,
				ImportStateVerify: true,
			},
			{Config: ReadFixture(t, "service_binding_auto_hints", "no_auto.tf"), ConfigPlanChecks: expectUpdate("technitium_record_https.www")},
			{Config: ReadFixture(t, "service_binding_auto_hints", "update.tf"), ConfigPlanChecks: expectUpdate("technitium_record_https.www")},
			{
				Config: ReadFixture(t, "service_binding_auto_hints", "zone_only.tf"),
				Check: func(*terraform.State) error {
					if recs := records(t, "hints.test", "www.hints.test", "HTTPS"); len(recs) != 0 {
						return fmt.Errorf("HTTPS record left behind: %v", recs)
					}
					return nil
				},
			},
		},
	})
}

// TestIntegration_DSAlgorithmSpelling uses the names the server writes with underscores.
func TestIntegration_DSAlgorithmSpelling(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("ds.test"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, "ds_algorithm_spelling", "main.tf")},
			{ResourceName: "technitium_record_ds.gost", ImportState: true, ImportStateVerify: true},
		},
	})
}

// TestIntegration_FWDRecordHasNoTTL: the server keeps every FWD record at TTL 0.
func TestIntegration_FWDRecordHasNoTTL(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("fwdttl.test"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, "fwd_record_has_no_ttl", "with_ttl.tf"), ExpectError: regexp.MustCompile(`Unsupported argument`)},
			{Config: ReadFixture(t, "fwd_record_has_no_ttl", "main.tf")},
			{ResourceName: "technitium_record_fwd.test", ImportState: true, ImportStateVerify: true},
		},
	})
}

func TestIntegration_RecordDefaults(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("defaults.test"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, "record_defaults", "main.tf")},
			{
				Config:           ReadFixture(t, "record_defaults", "defaults.tf"),
				ConfigPlanChecks: expectUpdate("technitium_record_a.www"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_a.www", "disable", "false"),
					resource.TestCheckResourceAttr("technitium_record_a.www", "comments", ""),
					resource.TestCheckResourceAttr("technitium_record_a.www", "expiry_ttl", "0"),
				),
			},
			{Config: ReadFixture(t, "record_defaults", "comments.tf")},
			{
				PreConfig: func() {
					call(t, "/api/zones/records/update", url.Values{"zone": {"defaults.test"}, "domain": {"www.defaults.test"}, "type": {"A"}, "ipAddress": {"10.0.0.1"}, "comments": {""}})
				},
				Config:           ReadFixture(t, "record_defaults", "comments.tf"),
				ConfigPlanChecks: expectUpdate("technitium_record_a.www"),
			},
			{ResourceName: "technitium_record_a.www", ImportState: true, ImportStateVerify: true},
		},
	})
}

func TestIntegration_AddressWithExplicitPTR(t *testing.T) {
	ptrs := func(want int) resource.TestCheckFunc {
		return func(*terraform.State) error {
			if got := records(t, "100.51.198.in-addr.arpa", "7.100.51.198.in-addr.arpa", "PTR"); len(got) != want {
				return fmt.Errorf("want %d PTR records, got %v", want, got)
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("ptr.test", "100.51.198.in-addr.arpa"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, "address_with_explicit_ptr", "main.tf"), Check: ptrs(1)},
			{Config: ReadFixture(t, "address_with_explicit_ptr", "update.tf"), ConfigPlanChecks: expectUpdate("technitium_record_a.www"), Check: ptrs(1)},
			// Replacing the A record deletes the PTR on the server; replace_triggered_by puts it back.
			{Config: ReadFixture(t, "address_with_explicit_ptr", "replace.tf"), Check: ptrs(1)},
			{Config: ReadFixture(t, "address_with_explicit_ptr", "replace.tf"), PlanOnly: true},
		},
	})
}

func TestIntegration_APPRecordOnePerName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("appone.test"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, "app_record_one_per_name", "main.tf"), ExpectError: regexp.MustCompile(`already exists`)},
		},
	})
}

func TestIntegration_DHCPScopeExists(t *testing.T) {
	acceptance(t)
	call(t, "/api/dhcp/scopes/set", url.Values{"name": {"taken"}, "startingAddress": {"192.168.60.10"}, "endingAddress": {"192.168.60.20"}, "subnetMask": {"255.255.255.0"}})
	t.Cleanup(func() {
		_, err := api().Call(context.Background(), "/api/dhcp/scopes/delete", url.Values{"name": {"taken"}})
		require.NoError(t, err)
	})
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:      ReadFixture(t, "dhcp_scope_exists", "main.tf"),
				ExpectError: regexp.MustCompile(`already exists`),
			},
		},
	})
}

func TestIntegration_ServiceBindingParamSpelling(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("svcparams.test"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, "service_binding_param_spelling", "main.tf")},
			{Config: ReadFixture(t, "service_binding_param_spelling", "main.tf"), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
			{Config: ReadFixture(t, "service_binding_param_spelling", "update.tf"), ConfigPlanChecks: expectUpdate("technitium_record_https.www")},
		},
	})
}

func TestIntegration_ZoneCatalogSpelling(t *testing.T) {
	inCatalog := func(zone string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			opts := call(t, "/api/zones/options/get", url.Values{"zone": {zone}})
			if opts["catalog"] != "catalog.test" {
				return fmt.Errorf("%s catalog %v", zone, opts["catalog"])
			}
			return nil
		}
	}
	empty := resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("catalog.test", "created.test", "updated.test"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, "zone_catalog_spelling", "main.tf"), Check: inCatalog("created.test")},
			{Config: ReadFixture(t, "zone_catalog_spelling", "main.tf"), ConfigPlanChecks: empty},
			{Config: ReadFixture(t, "zone_catalog_spelling", "uncataloged.tf")},
			{Config: ReadFixture(t, "zone_catalog_spelling", "update.tf"), ConfigPlanChecks: expectUpdate("technitium_zone.updated"), Check: inCatalog("updated.test")},
			{Config: ReadFixture(t, "zone_catalog_spelling", "update.tf"), ConfigPlanChecks: empty},
		},
	})
}
