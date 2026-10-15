package technitium_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccAddressUpdatesSvcbHints: the server fills auto hints from the A
// records again only when the A call asks it to.
func TestAccAddressUpdatesSvcbHints(t *testing.T) {
	config := func(addrs ...string) string {
		out := `
resource "technitium_zone" "test" {
  zone = "hint.test"
}

resource "technitium_record_https" "www" {
  zone            = technitium_zone.test.zone
  domain          = "www.hint.test"
  svc_priority    = 1
  svc_target_name = "."
  svc_params      = { alpn = "h2" }
  auto_ipv4_hint  = true
  depends_on      = [technitium_record_a.first]
}

resource "technitium_record_a" "first" {
  zone       = technitium_zone.test.zone
  domain     = "www.hint.test"
  ip_address = "192.0.2.1"
}
`
		for i, a := range addrs {
			out += fmt.Sprintf(`
resource "technitium_record_a" "a%d" {
  zone       = technitium_zone.test.zone
  domain     = "www.hint.test"
  ip_address = %q
}
`, i, a)
		}
		return out
	}
	hint := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			got := records(t, "hint.test", "www.hint.test", "HTTPS")[0]["rData"].(map[string]any)["svcParams"].(map[string]any)["ipv4hint"]
			if got != want {
				return fmt.Errorf("ipv4hint %v, want %s", got, want)
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("hint.test"),
		Steps: []resource.TestStep{
			{Config: config(), Check: hint("192.0.2.1")},
			{Config: config("192.0.2.2"), Check: hint("192.0.2.1,192.0.2.2")},
			{Config: config("192.0.2.3"), Check: hint("192.0.2.1,192.0.2.3")},
		},
	})
}
