package technitium_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
)

func TestAccUserPasswordNotResentOnUnrelatedChange(t *testing.T) {
	acceptance(t)
	config := func(display string) string {
		return fmt.Sprintf(`
resource "technitium_user" "pw" {
  username     = "pwuser"
  password     = "first-secret"
  display_name = "%s"
}
`, display)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: config("One")},
			{
				PreConfig: func() {
					call(t, api(t), "/api/admin/users/set", url.Values{"user": {"pwuser"}, "newPass": {"oob-secret"}})
				},
				Config: config("Two"),
				Check: func(*terraform.State) error {
					c := client.New(os.Getenv("TECHNITIUM_HOST"), "", "test", false, nil)
					return c.Login(context.Background(), "pwuser", "oob-secret")
				},
			},
		},
	})
}
