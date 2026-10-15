package technitium_test

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccZoneDNSSECKeySizesAfterImport(t *testing.T) {
	zone := `
resource "technitium_zone" "test" {
  zone = "rsa.test"
}
`
	config := zone + `
import {
  to = technitium_zone_dnssec.test
  id = "rsa.test"
}

resource "technitium_zone_dnssec" "test" {
  zone           = technitium_zone.test.zone
  algorithm      = "RSA"
  hash_algorithm = "SHA256"
  ksk_key_size   = 2048
  zsk_key_size   = 1024
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("rsa.test"),
		Steps: []resource.TestStep{
			{Config: zone},
			{
				PreConfig: func() {
					call(t, api(t), "/api/zones/dnssec/sign", url.Values{"zone": {"rsa.test"}, "algorithm": {"RSA"},
						"hashAlgorithm": {"SHA256"}, "kskKeySize": {"2048"}, "zskKeySize": {"1024"}})
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_zone_dnssec.test", plancheck.ResourceActionNoop)},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "ksk_key_size", "2048"),
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "zsk_key_size", "1024"),
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: strings.Replace(config, "zsk_key_size   = 1024", "zsk_key_size   = 2048", 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_zone_dnssec.test", plancheck.ResourceActionDestroyBeforeCreate)},
				},
			},
		},
	})
}

func TestAccZoneDNSSECNSEC3KeepsOtherParam(t *testing.T) {
	config := func(params string) string {
		return `
resource "technitium_zone" "test" {
  zone = "nsec3.test"
}

resource "technitium_zone_dnssec" "test" {
  zone      = technitium_zone.test.zone
  algorithm = "ECDSA"
  curve     = "P256"
  nx_proof  = "NSEC3"
` + params + `
}
`
	}
	server := func(key, want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			resp := call(t, api(t), "/api/zones/dnssec/properties/get", url.Values{"zone": {"nsec3.test"}})
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
				Config: config(`nsec3_iterations = 1
  nsec3_salt_length = 8`),
				Check: server("nsec3SaltLength", "8"),
			},
			{
				Config: config(`nsec3_iterations = 5`),
				Check: resource.ComposeTestCheckFunc(
					server("nsec3Iterations", "5"),
					server("nsec3SaltLength", "8"),
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "nsec3_salt_length", "8"),
				),
			},
			{
				Config: config(`nsec3_salt_length = 4`),
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
func TestAccZoneDNSSECDestroyWithRecords(t *testing.T) {
	var config string
	for _, nx := range []string{"NSEC", "NSEC3"} {
		name := strings.ToLower(nx)
		config += fmt.Sprintf(`
resource "technitium_zone" "%[1]s" {
  zone = "%[1]s-destroy.test"
}

resource "technitium_zone_dnssec" "%[1]s" {
  zone      = technitium_zone.%[1]s.zone
  algorithm = "ECDSA"
  curve     = "P256"
  nx_proof  = "%[2]s"
}

resource "technitium_record_a" "%[1]s" {
  count      = 12
  zone       = technitium_zone.%[1]s.zone
  domain     = "host${count.index}.%[1]s-destroy.test"
  ip_address = "10.0.0.${count.index + 1}"
}
`, name, nx)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("nsec-destroy.test", "nsec3-destroy.test"),
		Steps:                    []resource.TestStep{{Config: config}},
	})
}
