package technitium_test

import (
	"context"
	"net/url"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/assert"
)

// The server stores an IP address under its reverse name, and adding an
// entry again succeeds without a change.
func TestAccSideListedZoneServer(t *testing.T) {
	acceptance(t)
	c := api(t)
	ctx := context.Background()
	for _, kind := range []string{"allowed", "blocked"} {
		t.Cleanup(func() {
			_, _ = c.Call(ctx, "/api/"+kind+"/delete", url.Values{"domain": {"5.1.168.192.in-addr.arpa"}})
			_, _ = c.Call(ctx, "/api/"+kind+"/delete", url.Values{"domain": {"side-listed.test"}})
		})
		call(t, c, "/api/"+kind+"/add", url.Values{"domain": {"192.168.1.5"}})
		resp := call(t, c, "/api/"+kind+"/list", url.Values{"domain": {"5.1.168.192.in-addr.arpa"}})
		assert.Equal(t, "5.1.168.192.in-addr.arpa", resp["domain"], kind)
		assert.NotEmpty(t, resp["records"], kind)

		call(t, c, "/api/"+kind+"/add", url.Values{"domain": {"side-listed.test"}})
		call(t, c, "/api/"+kind+"/add", url.Values{"domain": {"side-listed.test"}})
	}
}

// IP addresses and networks are refused at plan time, and an existing entry
// on create.
func TestAccSideListedZoneRefused(t *testing.T) {
	acceptance(t)
	c := api(t)
	ctx := context.Background()
	call(t, c, "/api/blocked/add", url.Values{"domain": {"side-existing.test"}})
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/blocked/delete", url.Values{"domain": {"side-existing.test"}})
	})
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:      `resource "technitium_allowed_zone" "ip" { domain = "192.168.1.6" }`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Domain is an IP address`),
			},
			{
				Config:      `resource "technitium_blocked_zone" "net" { domain = "10.0.0.0/8" }`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`10.in-addr.arpa`),
			},
			{
				Config:      `resource "technitium_blocked_zone" "existing" { domain = "side-existing.test" }`,
				ExpectError: regexp.MustCompile(`blocked_zone already exists; import it`),
			},
			{
				Config: `resource "technitium_blocked_zone" "sub" { domain = "sub.side-existing.test" }`,
				Check:  resource.TestCheckResourceAttr("technitium_blocked_zone.sub", "domain", "sub.side-existing.test"),
			},
		},
	})
}
