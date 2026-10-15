package technitium_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/stretchr/testify/require"
)

func TestAccRecordTTLAndCommentDrift(t *testing.T) {
	config := `
resource "technitium_zone" "test" {
  zone = "drift.test"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.drift.test"
  ip_address = "10.0.0.1"
  ttl        = 300
  comments   = "managed"
}
`
	const addr = "technitium_record_a.www"
	change := func(ttl, comments string) func() {
		return func() {
			_, err := api(t).Call(context.Background(), "/api/zones/records/update", url.Values{
				"zone": {"drift.test"}, "domain": {"www.drift.test"}, "type": {"A"}, "ipAddress": {"10.0.0.1"},
				"ttl": {ttl}, "comments": {comments},
			})
			require.NoError(t, err)
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("drift.test"),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "ttl", "300"),
					resource.TestCheckResourceAttr(addr, "comments", "managed"),
				),
			},
			{
				PreConfig:        change("600", "managed"),
				Config:           config,
				ConfigPlanChecks: expectUpdate(addr),
				Check:            resource.TestCheckResourceAttr(addr, "ttl", "300"),
			},
			{
				PreConfig:        change("300", "edited by hand"),
				Config:           config,
				ConfigPlanChecks: expectUpdate(addr),
				Check:            resource.TestCheckResourceAttr(addr, "comments", "managed"),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestAccZoneDNSSECUnsignedOutOfBand(t *testing.T) {
	config := `
resource "technitium_zone" "test" {
  zone = "unsigned-oob.test"
}

resource "technitium_zone_dnssec" "test" {
  zone      = technitium_zone.test.zone
  algorithm = "ECDSA"
  curve     = "P256"
  nx_proof  = "NSEC3"
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("unsigned-oob.test"),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "status", "SignedWithNSEC3"),
			},
			{
				PreConfig: func() {
					call(t, api(t), "/api/zones/dnssec/unsign", url.Values{"zone": {"unsigned-oob.test"}})
				},
				Config:           config,
				ConfigPlanChecks: expectCreate("technitium_zone_dnssec.test"),
				Check:            resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "status", "SignedWithNSEC3"),
			},
		},
	})
}
