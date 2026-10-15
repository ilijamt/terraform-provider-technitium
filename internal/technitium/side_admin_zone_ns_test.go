package technitium_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A new Primary zone gets an apex NS record naming the server, also one
// converted from a Forwarder zone.
func TestAccSideZonePrimaryAddsNS(t *testing.T) {
	acceptance(t)
	c := api(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-ns.test"}})
		_, _ = c.Call(ctx, "/api/zones/delete", url.Values{"zone": {"side-fwd.test"}})
	})
	ns := func(zone string) []string {
		resp := call(t, c, "/api/zones/records/get", url.Values{"zone": {zone}, "domain": {zone}})
		var out []string
		for _, r := range resp["records"].([]any) {
			rec := r.(map[string]any)
			if rec["type"] == "NS" {
				out = append(out, rec["rData"].(map[string]any)["nameServer"].(string))
			}
		}
		return out
	}
	general := call(t, c, "/api/settings/get", nil)
	server, _ := general["dnsServerDomain"].(string)
	require.NotEmpty(t, server)

	call(t, c, "/api/zones/create", url.Values{"zone": {"side-ns.test"}})
	assert.Equal(t, []string{server}, ns("side-ns.test"))

	call(t, c, "/api/zones/create", url.Values{"zone": {"side-fwd.test"}, "type": {"Forwarder"}, "initializeForwarder": {"false"}})
	call(t, c, "/api/zones/convert", url.Values{"zone": {"side-fwd.test"}, "type": {"Primary"}})
	assert.Equal(t, []string{server}, ns("side-fwd.test"))
}
