//go:build integration

package examples

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"
)

// TestIntegration_SignedZoneRefusesRecords: a signed zone refuses disabled,
// ANAME and APP records, and a zone holding one cannot be signed.
func TestIntegration_SignedZoneRefusesRecords(t *testing.T) {
	acceptance(t)
	c := api()
	const zone = "refuse.test"
	call(t, "/api/zones/create", url.Values{"zone": {zone}, "type": {"Primary"}})
	t.Cleanup(func() { _, _ = c.Call(context.Background(), "/api/zones/delete", url.Values{"zone": {zone}}) })
	sign := url.Values{"zone": {zone}, "algorithm": {"ECDSA"}, "curve": {"P256"}}
	// The add call takes no disable, so the A record is disabled by update.
	disabled := url.Values{"type": {"A"}, "ipAddress": {"192.0.2.1"}}
	recs := []url.Values{
		disabled,
		{"type": {"ANAME"}, "aname": {"target.example"}},
		{"type": {"APP"}, "appName": {"Split Horizon"}, "classPath": {"SplitHorizon.SimpleAddress"}},
	}
	at := func(r url.Values) url.Values {
		return merge(r, url.Values{"zone": {zone}, "domain": {"www." + zone}})
	}

	for _, r := range recs {
		call(t, "/api/zones/records/add", at(r))
		if r.Get("type") == "A" {
			call(t, "/api/zones/records/update", merge(at(r), url.Values{"disable": {"true"}}))
		}
		_, err := c.Call(context.Background(), "/api/zones/dnssec/sign", sign)
		require.ErrorContains(t, err, "Cannot sign RRSet", r.Get("type"))
		call(t, "/api/zones/records/delete", at(r))
	}

	call(t, "/api/zones/dnssec/sign", sign)
	for _, r := range recs[1:] {
		_, err := c.Call(context.Background(), "/api/zones/records/add", at(r))
		require.ErrorContains(t, err, "not supported by DNSSEC signed", r.Get("type"))
	}
	call(t, "/api/zones/records/add", at(disabled))
	_, err := c.Call(context.Background(), "/api/zones/records/update", merge(at(disabled), url.Values{"disable": {"true"}}))
	require.ErrorContains(t, err, "disabling records in a signed zones is not supported")
	call(t, "/api/zones/dnssec/unsign", url.Values{"zone": {zone}})
}

func TestIntegration_ZoneDNSSECKeySizeMismatchOnImport(t *testing.T) {
	const name = "zone_dnssec_key_size_mismatch_on_import"
	zskSize := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			props := call(t, "/api/zones/dnssec/properties/get", url.Values{"zone": {"rsasize.test"}})
			for _, k := range props["dnssecPrivateKeys"].([]any) {
				m := k.(map[string]any)
				if m["keyType"] == "ZoneSigningKey" && m["algorithm"] != "RSASHA256 ("+want+" bits)" {
					return fmt.Errorf("zone signing key %v", m["algorithm"])
				}
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("rsasize.test"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, name, "main.tf")},
			{
				Config:      ReadFixture(t, name, "ecdsa_key_size.tf"),
				ExpectError: regexp.MustCompile(`only applies to RSA`),
			},
			{
				PreConfig: func() {
					call(t, "/api/zones/dnssec/sign", url.Values{"zone": {"rsasize.test"}, "algorithm": {"RSA"},
						"hashAlgorithm": {"SHA256"}, "kskKeySize": {"2048"}, "zskKeySize": {"1024"}})
				},
				Config:           ReadFixture(t, name, "import.tf"),
				ConfigPlanChecks: expectReplace("technitium_zone_dnssec.test"),
				Check: resource.ComposeTestCheckFunc(
					zskSize("2048"),
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "zsk_key_size", "2048"),
				),
			},
			{
				Config:           ReadFixture(t, name, "import_default.tf"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check:            resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "ksk_key_size", "2048"),
			},
		},
	})
}

func TestIntegration_ZoneDNSSECUnsignedOutOfBand(t *testing.T) {
	config := ReadFixture(t, "zone_dnssec_unsigned_out_of_band", "main.tf")
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
					call(t, "/api/zones/dnssec/unsign", url.Values{"zone": {"unsigned-oob.test"}})
				},
				Config:           config,
				ConfigPlanChecks: expectCreate("technitium_zone_dnssec.test"),
				Check:            resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "status", "SignedWithNSEC3"),
			},
		},
	})
}
