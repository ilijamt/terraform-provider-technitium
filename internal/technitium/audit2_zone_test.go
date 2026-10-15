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

func TestAccZoneSuggestsReverseName(t *testing.T) {
	for name, want := range map[string]string{
		"192.168.1.0/24": `"1.168.192.in-addr.arpa"`,
		"192.168.1.1":    `"1.1.168.192.in-addr.arpa"`,
		"2001:db8::/32":  `"8.b.d.0.1.0.0.2.ip6.arpa"`,
	} {
		for _, kind := range []string{"resource", "data"} {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{{
					Config:      fmt.Sprintf(`%s "technitium_zone" "test" { zone = %q }`, kind, name),
					ExpectError: regexp.MustCompile(regexp.QuoteMeta(want)),
				}},
			})
		}
	}
}

func TestAccZoneTypeConverts(t *testing.T) {
	config := func(typ string) string {
		return fmt.Sprintf(`
resource "technitium_zone" "test" {
  zone         = "convert2.test"
  type         = %q
  query_access = "AllowOnlyPrivateNetworks"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.convert2.test"
  ip_address = "10.0.0.1"
}
`, typ)
	}
	recordKept := func(*terraform.State) error {
		resp := call(t, api(t), "/api/zones/records/get", url.Values{"zone": {"convert2.test"}, "domain": {"www.convert2.test"}})
		if records, _ := resp["records"].([]any); len(records) != 1 {
			return fmt.Errorf("want the record kept, got %v", records)
		}
		return nil
	}
	inPlace := func(typ string) resource.TestStep {
		return resource.TestStep{
			Config: config(typ),
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
			{Config: config("Primary")},
			inPlace("Forwarder"),
			inPlace("Primary"),
			{
				Config: config("Catalog"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_zone.test", plancheck.ResourceActionDestroyBeforeCreate)},
				},
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
