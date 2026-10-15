package technitium_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccRecordsShareTTL: the server keeps one TTL per name and type and
// rewrites the others, dropping their comments, expiry and disabled flag, so
// a differing TTL is refused, an unset one takes theirs, and a change on a
// record that had the shared TTL moves them all and keeps their values.
func TestAccRecordsShareTTL(t *testing.T) {
	config := func(a, b string) string {
		return fmt.Sprintf(`
resource "technitium_zone" "test" {
  zone = "ttl.test"
}

resource "technitium_record_a" "a" {
  zone       = technitium_zone.test.zone
  domain     = "www.ttl.test"
  ip_address = "192.0.2.1"
  ttl        = %s
}

resource "technitium_record_a" "b" {
  zone       = technitium_zone.test.zone
  domain     = "www.ttl.test"
  ip_address = "192.0.2.2"
  ttl        = %s
  comments   = "keep"
  expiry_ttl = 86400
  disable    = true
  depends_on = [technitium_record_a.a]
}
`, a, b)
	}
	kept := func(ttl int) resource.TestCheckFunc {
		return func(*terraform.State) error {
			for _, r := range records(t, "ttl.test", "www.ttl.test", "A") {
				if fmt.Sprint(r["ttl"]) != fmt.Sprint(ttl) {
					return fmt.Errorf("ttl %v, want %v", r["ttl"], ttl)
				}
				if r["rData"].(map[string]any)["ipAddress"] == "192.0.2.2" &&
					(r["comments"] != "keep" || r["disabled"] != true || fmt.Sprint(r["expiryTtl"]) != "86400") {
					return fmt.Errorf("values of b lost: %v", r)
				}
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("ttl.test"),
		Steps: []resource.TestStep{
			{
				Config:      config("300", "600"),
				ExpectError: regexp.MustCompile(`records at www.ttl.test of type A share one ttl \(300\); set ttl = 300`),
			},
			{
				Config: config("300", "null"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_a.b", "ttl", "300"),
					kept(300),
				),
			},
			{Config: config("900", "null"), Check: kept(900)},
			{Config: config("60", "60"), Check: kept(60)},
		},
	})
}
