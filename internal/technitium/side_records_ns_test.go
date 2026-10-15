package technitium_test

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAccNSDefaultTTL: an NS record without a TTL takes the default NS
// record TTL (14400), not the default record TTL (3600).
func TestAccNSDefaultTTL(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("nsttl.test"),
		Steps: []resource.TestStep{{
			Config: `
resource "technitium_zone" "test" {
  zone = "nsttl.test"
}

resource "technitium_record_ns" "sub" {
  zone        = technitium_zone.test.zone
  domain      = "sub.nsttl.test"
  name_server = "ns1.example.net"
}
`,
			Check: resource.TestCheckResourceAttr("technitium_record_ns.sub", "ttl", "14400"),
		}},
	})
}

func TestNSTTLDescription(t *testing.T) {
	srv, err := factories["technitium"]()
	require.NoError(t, err)
	resp, err := srv.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	require.NoError(t, err)
	for _, a := range resp.ResourceSchemas["technitium_record_ns"].Block.Attributes {
		if a.Name == "ttl" {
			assert.Contains(t, a.Description, "default NS record TTL")
		}
	}
}

// TestAccNSApexOfNewZone: a new primary zone holds an apex NS record named
// after the server domain. Creating the same record is refused whatever its
// TTL, since the server would take it over with the new TTL; import it.
func TestAccNSApexOfNewZone(t *testing.T) {
	acceptance(t)
	server := call(t, api(t), "/api/settings/get", url.Values{})["dnsServerDomain"].(string)
	config := func(ttl string) string {
		return fmt.Sprintf(`
resource "technitium_zone" "test" {
  zone = "nsapex.test"
}

resource "technitium_record_ns" "apex" {
  zone        = technitium_zone.test.zone
  domain      = "nsapex.test"
  name_server = %q
  ttl         = %s
}
`, server, ttl)
	}
	refused := regexp.MustCompile(`record_ns already exists; import it to manage it`)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("nsapex.test"),
		Steps: []resource.TestStep{
			{Config: config("14400"), ExpectError: refused},
			{Config: config("300"), ExpectError: refused},
			{
				Config:             config("300"),
				ResourceName:       "technitium_record_ns.apex",
				ImportState:        true,
				ImportStateId:      "nsapex.test/nsapex.test/" + server,
				ImportStatePersist: true,
			},
			{
				Config: config("300"),
				Check: func(*terraform.State) error {
					if ttl := fmt.Sprint(records(t, "nsapex.test", "nsapex.test", "NS")[0]["ttl"]); ttl != "300" {
						return fmt.Errorf("ttl %s, want 300", ttl)
					}
					return nil
				},
			},
		},
	})
}
