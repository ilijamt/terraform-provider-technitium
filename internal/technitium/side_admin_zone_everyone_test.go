package technitium_test

import (
	"context"
	"net/url"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every new user joins Everyone, and the server refuses to take anyone out
// of it.
func TestAccSideEveryoneMembers(t *testing.T) {
	acceptance(t)
	c := api(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/admin/users/delete", url.Values{"user": {"sideeveryone"}})
	})
	members := func() []string {
		return names(call(t, c, "/api/admin/groups/get", url.Values{"group": {"Everyone"}})["members"], "")
	}
	call(t, c, "/api/admin/users/create", url.Values{"user": {"sideeveryone"}, "pass": {"sideeveryone-pass"}})
	require.Contains(t, members(), "sideeveryone")

	_, err := c.Call(ctx, "/api/admin/groups/set", url.Values{"group": {"Everyone"}, "members": {"admin"}})
	require.ErrorContains(t, err, "Access was denied")
	assert.Contains(t, members(), "sideeveryone")
}

// Everyone's members are refused at plan time; the data source still reads
// them.
func TestAccSideEveryoneRefused(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "technitium_group_members" "everyone" {
  group   = "everyone"
  members = ["admin"]
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("the Everyone group cannot be managed"),
			},
			{
				Config: `
data "technitium_group_members" "everyone" {
  group = "Everyone"
}
`,
				Check: resource.TestCheckTypeSetElemAttr("data.technitium_group_members.everyone", "members.*", "admin"),
			},
		},
	})
}
