//go:build integration

package examples

import (
	"fmt"
	"net/url"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestIntegration_ZoneSuggestsReverseName(t *testing.T) {
	for name, want := range map[string]string{
		"192.168.1.0/24": `"1.168.192.in-addr.arpa"`,
		"192.168.1.1":    `"1.1.168.192.in-addr.arpa"`,
		"2001:db8::/32":  `"8.b.d.0.1.0.0.2.ip6.arpa"`,
	} {
		for _, file := range []string{"resource.tf", "data.tf"} {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{{
					Config:          ReadFixture(t, "zone_suggests_reverse_name", file),
					ConfigVariables: config.Variables{"zone": config.StringVariable(name)},
					ExpectError:     regexp.MustCompile(regexp.QuoteMeta(want)),
				}},
			})
		}
	}
}

func TestIntegration_ZoneTypeConverts(t *testing.T) {
	recordKept := func(*terraform.State) error {
		resp := call(t, "/api/zones/records/get", url.Values{"zone": {"convert2.test"}, "domain": {"www.convert2.test"}})
		if records, _ := resp["records"].([]any); len(records) != 1 {
			return fmt.Errorf("want the record kept, got %v", records)
		}
		return nil
	}
	inPlace := func(file, typ string) resource.TestStep {
		return resource.TestStep{
			Config: ReadFixture(t, "zone_type_converts", file),
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("technitium_zone.test", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("technitium_record_a.www", plancheck.ResourceActionNoop),
				},
			},
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("technitium_zone.test", "type", typ),
				resource.TestCheckResourceAttr("technitium_zone.test", "query_access", "AllowOnlyPrivateNetworks"),
				recordKept,
			),
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("convert2.test"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, "zone_type_converts", "main.tf")},
			inPlace("forwarder.tf", "Forwarder"),
			inPlace("main.tf", "Primary"),
			{
				Config:             ReadFixture(t, "zone_type_converts", "catalog.tf"),
				ConfigPlanChecks:   expectReplace("technitium_zone.test"),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestIntegration_ZoneDNSSECKeySizesAfterImport(t *testing.T) {
	const dir = "zone_dnssec_key_sizes_after_import"
	main := ReadFixture(t, dir, "main.tf")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("rsa.test"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, dir, "zone.tf")},
			{
				PreConfig: func() {
					call(t, "/api/zones/dnssec/sign", url.Values{"zone": {"rsa.test"}, "algorithm": {"RSA"},
						"hashAlgorithm": {"SHA256"}, "kskKeySize": {"2048"}, "zskKeySize": {"1024"}})
				},
				Config:           main,
				ConfigPlanChecks: expectNoop("technitium_zone_dnssec.test"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "ksk_key_size", "2048"),
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "zsk_key_size", "1024"),
				),
			},
			{
				Config: main,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config:           ReadFixture(t, dir, "zsk_2048.tf"),
				ConfigPlanChecks: expectReplace("technitium_zone_dnssec.test"),
			},
		},
	})
}

func TestIntegration_ZoneDNSSECNSEC3KeepsOtherParam(t *testing.T) {
	const dir = "zone_dnssec_nsec3_keeps_other_param"
	server := func(key, want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			resp := call(t, "/api/zones/dnssec/properties/get", url.Values{"zone": {"nsec3.test"}})
			if got := fmt.Sprint(resp[key]); got != want {
				return fmt.Errorf("server %s = %s, want %s", key, got, want)
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("nsec3.test"),
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, dir, "main.tf"),
				Check:  server("nsec3SaltLength", "8"),
			},
			{
				Config: ReadFixture(t, dir, "iterations.tf"),
				Check: resource.ComposeTestCheckFunc(
					server("nsec3Iterations", "5"),
					server("nsec3SaltLength", "8"),
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "nsec3_salt_length", "8"),
				),
			},
			{
				Config: ReadFixture(t, dir, "salt.tf"),
				Check: resource.ComposeTestCheckFunc(
					server("nsec3Iterations", "5"),
					server("nsec3SaltLength", "4"),
				),
			},
		},
	})
}

// Terraform unsigns a zone and deletes its records at once, since each
// depends only on the zone.
func TestIntegration_ZoneDNSSECDestroyWithRecords(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("nsec-destroy.test", "nsec3-destroy.test"),
		Steps:                    []resource.TestStep{{Config: ReadFixture(t, "zone_dnssec_destroy_with_records", "main.tf")}},
	})
}
