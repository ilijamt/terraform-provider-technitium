//go:build integration

package examples

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"
)

// zoneApex returns the SOA rData and the NS name servers at zone's apex.
func zoneApex(zone string) (map[string]any, []string, error) {
	resp, err := api().Call(context.Background(), "/api/zones/records/get", url.Values{"zone": {zone}, "domain": {zone}})
	if err != nil {
		return nil, nil, err
	}
	var soa map[string]any
	var ns []string
	for _, r := range resp["records"].([]any) {
		rec := r.(map[string]any)
		switch rec["type"] {
		case "SOA":
			soa = rec["rData"].(map[string]any)
		case "NS":
			ns = append(ns, rec["rData"].(map[string]any)["nameServer"].(string))
		}
	}
	return soa, ns, nil
}

// TFTECH-108: changing dns_server_domain rewrites the SOA and apex NS of
// primary zones that named the old domain, and drops the SOA's serial date
// scheme.
func TestIntegration_SideSettingsServerDomainRewritesZones(t *testing.T) {
	acceptance(t)
	const name = "side_settings_server_domain_rewrites_zones"
	old := call(t, "/api/settings/get", nil)["dnsServerDomain"].(string)
	t.Cleanup(func() { call(t, "/api/settings/set", url.Values{"dnsServerDomain": {old}}) })

	rewritten := func(*terraform.State) error {
		var soa map[string]any
		var ns []string
		var err error
		for range 20 {
			soa, ns, err = zoneApex("side108.test")
			if err == nil && soa["primaryNameServer"] == "ns.side108.test" {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if err != nil {
			return err
		}
		if soa["primaryNameServer"] != "ns.side108.test" || len(ns) != 1 || ns[0] != "ns.side108.test" {
			return fmt.Errorf("SOA %v and NS %v were not rewritten", soa["primaryNameServer"], ns)
		}
		if soa["useSerialDateScheme"] != false {
			return fmt.Errorf("useSerialDateScheme is %v", soa["useSerialDateScheme"])
		}
		return nil
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: ReadFixture(t, name, "main.tf"),
				Check: func(*terraform.State) error {
					soa, ns, err := zoneApex("side108.test")
					require.NoError(t, err)
					require.Equal(t, old, soa["primaryNameServer"])
					require.Equal(t, []string{old}, ns)
					require.Equal(t, true, soa["useSerialDateScheme"])
					return nil
				},
			},
			{
				Config:             ReadFixture(t, name, "domain.tf"),
				Check:              rewritten,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
