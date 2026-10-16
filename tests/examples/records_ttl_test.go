//go:build integration

package examples

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"
)

// TestIntegration_RecordsShareTTL: the server keeps one TTL per name and type
// and rewrites the others, dropping their comments, expiry and disabled flag,
// so a differing TTL is refused, an unset one takes theirs, and a change on a
// record that had the shared TTL moves them all and keeps their values.
func TestIntegration_RecordsShareTTL(t *testing.T) {
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
				Config:      ReadFixture(t, "records_share_ttl", "conflict.tf"),
				ExpectError: regexp.MustCompile(`records at www.ttl.test of type A share one ttl \(300\); set ttl = 300`),
			},
			{
				Config: ReadFixture(t, "records_share_ttl", "main.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_a.b", "ttl", "300"),
					kept(300),
				),
			},
			{Config: ReadFixture(t, "records_share_ttl", "change.tf"), Check: kept(900)},
			{Config: ReadFixture(t, "records_share_ttl", "both.tf"), Check: kept(60)},
		},
	})
}

// TestIntegration_NSDefaultTTL: an NS record without a TTL takes the default
// NS record TTL (14400), not the default record TTL (3600).
func TestIntegration_NSDefaultTTL(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("nsttl.test"),
		Steps: []resource.TestStep{{
			Config: ReadFixture(t, "ns_default_ttl", "main.tf"),
			Check:  resource.TestCheckResourceAttr("technitium_record_ns.sub", "ttl", "14400"),
		}},
	})
}

// TestIntegration_NSApexOfNewZone: a new primary zone holds an apex NS record
// named after the server domain. Creating the same record is refused whatever
// its TTL, since the server would take it over with the new TTL; import it.
func TestIntegration_NSApexOfNewZone(t *testing.T) {
	acceptance(t)
	server := call(t, "/api/settings/get", url.Values{})["dnsServerDomain"].(string)
	vars := config.Variables{"server": config.StringVariable(server)}
	refused := regexp.MustCompile(`record_ns already exists; import it to manage it`)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("nsapex.test"),
		Steps: []resource.TestStep{
			{Config: ReadFixture(t, "ns_apex_of_new_zone", "main.tf"), ConfigVariables: vars, ExpectError: refused},
			{Config: ReadFixture(t, "ns_apex_of_new_zone", "ttl.tf"), ConfigVariables: vars, ExpectError: refused},
			{
				Config:             ReadFixture(t, "ns_apex_of_new_zone", "ttl.tf"),
				ConfigVariables:    vars,
				ResourceName:       "technitium_record_ns.apex",
				ImportState:        true,
				ImportStateId:      "nsapex.test/nsapex.test/" + server,
				ImportStatePersist: true,
			},
			{
				Config:          ReadFixture(t, "ns_apex_of_new_zone", "ttl.tf"),
				ConfigVariables: vars,
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

// TestIntegration_AddressUpdatesSvcbHints: the server fills auto hints from
// the A records again only when the A call asks it to.
func TestIntegration_AddressUpdatesSvcbHints(t *testing.T) {
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
			{Config: ReadFixture(t, "address_updates_svcb_hints", "main.tf"), Check: hint("192.0.2.1")},
			{Config: ReadFixture(t, "address_updates_svcb_hints", "add.tf"), Check: hint("192.0.2.1,192.0.2.2")},
			{Config: ReadFixture(t, "address_updates_svcb_hints", "change.tf"), Check: hint("192.0.2.1,192.0.2.3")},
		},
	})
}

func TestIntegration_RecordTTLAndCommentDrift(t *testing.T) {
	cfg := ReadFixture(t, "record_ttl_and_comment_drift", "main.tf")
	const addr = "technitium_record_a.www"
	change := func(ttl, comments string) func() {
		return func() {
			_, err := api().Call(context.Background(), "/api/zones/records/update", url.Values{
				"zone": {"drift-oob.test"}, "domain": {"www.drift-oob.test"}, "type": {"A"}, "ipAddress": {"10.0.0.1"},
				"ttl": {ttl}, "comments": {comments},
			})
			require.NoError(t, err)
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("drift-oob.test"),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "ttl", "300"),
					resource.TestCheckResourceAttr(addr, "comments", "managed"),
				),
			},
			{
				PreConfig:        change("600", "managed"),
				Config:           cfg,
				ConfigPlanChecks: expectUpdate(addr),
				Check:            resource.TestCheckResourceAttr(addr, "ttl", "300"),
			},
			{
				PreConfig:        change("300", "edited by hand"),
				Config:           cfg,
				ConfigPlanChecks: expectUpdate(addr),
				Check:            resource.TestCheckResourceAttr(addr, "comments", "managed"),
			},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
