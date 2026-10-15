package technitium_test

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccZoneLeaveCatalog(t *testing.T) {
	config := func(catalog string) string {
		return fmt.Sprintf(`
resource "technitium_zone" "catalog" {
  zone = "leave-catalog.test"
  type = "Catalog"
}

resource "technitium_zone" "member" {
  zone       = "leave-member.test"
  %s
  depends_on = [technitium_zone.catalog]
}
`, catalog)
	}
	member := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			opts := call(t, api(t), "/api/zones/options/get", url.Values{"zone": {"leave-member.test"}})
			if got, _ := opts["catalog"].(string); got != want {
				return fmt.Errorf("catalog %v, want %q", opts["catalog"], want)
			}
			return nil
		}
	}
	join := func() {
		call(t, api(t), "/api/zones/options/set", url.Values{"zone": {"leave-member.test"}, "catalog": {"leave-catalog.test"}})
	}
	in := config(`catalog = "leave-catalog.test"`)
	empty := resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("leave-catalog.test", "leave-member.test"),
		Steps: []resource.TestStep{
			{Config: config(""), Check: member("")},
			{Config: config(""), ConfigPlanChecks: empty},
			{Config: config(""), ResourceName: "technitium_zone.member", ImportState: true, ImportStateVerify: true},
			{Config: in, Check: member("leave-catalog.test")},
			{Config: config(""), ConfigPlanChecks: expectUpdate("technitium_zone.member"), Check: member("")},
			{Config: config(""), ConfigPlanChecks: empty},
			{Config: in, Check: member("leave-catalog.test")},
			{Config: config(`catalog = ""`), ConfigPlanChecks: expectUpdate("technitium_zone.member"), Check: member("")},
			{Config: config(`catalog = ""`), ConfigPlanChecks: empty},
			{PreConfig: join, Config: config(""), ConfigPlanChecks: expectUpdate("technitium_zone.member"), Check: member("")},
			{Config: config(""), ConfigPlanChecks: empty},
		},
	})
}
