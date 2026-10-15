package technitium_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// The server drops users and groups it does not know from a permissions
// list without an error, so the provider reports them.
func TestAccSidePermissionsUnknownNames(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("side-unknown.test"),
		Steps: []resource.TestStep{
			{
				Config: `
resource "technitium_zone" "z" {
  zone = "side-unknown.test"
}

resource "technitium_zone_permissions" "z" {
  zone = technitium_zone.z.zone
  user_permissions = [
    { username = "nosuchuser", can_view = true, can_modify = false, can_delete = false },
  ]
}
`,
				ExpectError: regexp.MustCompile(`zone_permissions: user "nosuchuser" does not exist`),
			},
			{
				Config: `
resource "technitium_permissions_logs" "p" {
  group_permissions = [
    { name = "No Such Group", can_view = true, can_modify = false, can_delete = false },
  ]
}
`,
				ExpectError: regexp.MustCompile(`permissions_logs: group "No Such Group" does not\s+exist`),
			},
		},
	})
}
