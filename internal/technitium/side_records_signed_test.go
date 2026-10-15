package technitium_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAccSignedZoneRefusesRecords: a signed zone refuses disabled, ANAME and
// APP records, and a zone holding one cannot be signed.
func TestAccSignedZoneRefusesRecords(t *testing.T) {
	acceptance(t)
	c := api(t)
	const zone = "refuse.test"
	call(t, c, "/api/zones/create", url.Values{"zone": {zone}, "type": {"Primary"}})
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
		call(t, c, "/api/zones/records/add", at(r))
		if r.Get("type") == "A" {
			call(t, c, "/api/zones/records/update", merge(at(r), url.Values{"disable": {"true"}}))
		}
		_, err := c.Call(context.Background(), "/api/zones/dnssec/sign", sign)
		require.ErrorContains(t, err, "Cannot sign RRSet", r.Get("type"))
		call(t, c, "/api/zones/records/delete", at(r))
	}

	call(t, c, "/api/zones/dnssec/sign", sign)
	for _, r := range recs[1:] {
		_, err := c.Call(context.Background(), "/api/zones/records/add", at(r))
		require.ErrorContains(t, err, "not supported by DNSSEC signed", r.Get("type"))
	}
	call(t, c, "/api/zones/records/add", at(disabled))
	_, err := c.Call(context.Background(), "/api/zones/records/update", merge(at(disabled), url.Values{"disable": {"true"}}))
	require.ErrorContains(t, err, "disabling records in a signed zones is not supported")
	call(t, c, "/api/zones/dnssec/unsign", url.Values{"zone": {zone}})
}
