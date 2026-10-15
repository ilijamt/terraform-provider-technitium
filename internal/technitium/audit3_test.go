package technitium_test

import (
	"fmt"
	"net/url"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccServiceBindingParamSpelling(t *testing.T) {
	config := func(ttl int) string {
		return fmt.Sprintf(`
resource "technitium_zone" "test" {
  zone = "svcparams.test"
}

resource "technitium_record_https" "www" {
  zone            = technitium_zone.test.zone
  domain          = "www.svcparams.test"
  svc_priority    = 1
  svc_target_name = "."
  svc_params = {
    ipv4hint          = "10.0.0.2, 10.0.0.1"
    ipv6hint          = "2001:DB8::1,2001:db8:0:0::2"
    port              = "0443"
    alpn              = "h2, h3"
    mandatory         = "ALPN,port"
    ech               = "aabb"
    "no-default-alpn" = ""
  }
  ttl = %d
}
`, ttl)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("svcparams.test"),
		Steps: []resource.TestStep{
			{Config: config(300)},
			{Config: config(300), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
			{Config: config(600), ConfigPlanChecks: expectUpdate("technitium_record_https.www")},
		},
	})
}

func TestAccZoneCatalogSpelling(t *testing.T) {
	config := func(member, catalog string) string {
		return fmt.Sprintf(`
resource "technitium_zone" "catalog" {
  zone = "catalog.test"
  type = "Catalog"
}

resource "technitium_zone" "%s" {
  zone       = "%[1]s.test"
  catalog    = %s
  depends_on = [technitium_zone.catalog]
}
`, member, catalog)
	}
	inCatalog := func(zone string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			opts := call(t, api(t), "/api/zones/options/get", url.Values{"zone": {zone}})
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
			{Config: config("created", `"Catalog.TEST."`), Check: inCatalog("created.test")},
			{Config: config("created", `"Catalog.TEST."`), ConfigPlanChecks: empty},
			{Config: config("updated", "null")},
			{Config: config("updated", `"CATALOG.test."`), ConfigPlanChecks: expectUpdate("technitium_zone.updated"), Check: inCatalog("updated.test")},
			{Config: config("updated", `"CATALOG.test."`), ConfigPlanChecks: empty},
		},
	})
}

func TestAccZoneDNSSECKeySizeMismatchOnImport(t *testing.T) {
	zone := `
resource "technitium_zone" "test" {
  zone = "rsasize.test"
}
`
	config := func(sizes string) string {
		return zone + `
import {
  to = technitium_zone_dnssec.test
  id = "rsasize.test"
}

resource "technitium_zone_dnssec" "test" {
  zone           = technitium_zone.test.zone
  algorithm      = "RSA"
  hash_algorithm = "SHA256"
` + sizes + `
}
`
	}
	zskSize := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			props := call(t, api(t), "/api/zones/dnssec/properties/get", url.Values{"zone": {"rsasize.test"}})
			for _, k := range props["dnssecPrivateKeys"].([]any) {
				m := k.(map[string]any)
				if m["keyType"] == "ZoneSigningKey" && m["algorithm"] != "RSASHA256 ("+want+" bits)" {
					return fmt.Errorf("zone signing key %v", m["algorithm"])
				}
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("rsasize.test"),
		Steps: []resource.TestStep{
			{Config: zone},
			{
				Config: zone + `
resource "technitium_zone_dnssec" "test" {
  zone         = technitium_zone.test.zone
  algorithm    = "ECDSA"
  curve        = "P256"
  zsk_key_size = 2048
}
`,
				ExpectError: regexp.MustCompile(`only applies to RSA`),
			},
			{
				PreConfig: func() {
					call(t, api(t), "/api/zones/dnssec/sign", url.Values{"zone": {"rsasize.test"}, "algorithm": {"RSA"},
						"hashAlgorithm": {"SHA256"}, "kskKeySize": {"2048"}, "zskKeySize": {"1024"}})
				},
				Config: config("ksk_key_size = 2048\nzsk_key_size = 2048"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_zone_dnssec.test", plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.ComposeTestCheckFunc(
					zskSize("2048"),
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "zsk_key_size", "2048"),
				),
			},
			{
				Config:           config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check:            resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "ksk_key_size", "2048"),
			},
		},
	})
}
