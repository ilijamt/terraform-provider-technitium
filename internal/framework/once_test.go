package framework

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
)

// A record update locates the record by its old values, so a resend after
// the first one took effect fails or changes another record.
func TestRecordUpdateNotResent(t *testing.T) {
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		if r.URL.Path == "/api/zones/records/get" {
			_, _ = w.Write([]byte(`{"status":"ok","response":{"records":[]}}`))
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		require.NoError(t, err)
		_ = conn.Close()
	}))
	defer srv.Close()

	spec := Record("A", Attr{Name: "ip_address", Param: "ipAddress", Type: String, Key: "rData.ipAddress", Required: true, Identity: true})
	r := &Resource{spec: spec, client: client.New(srv.URL, "t0k", "test", false, nil)}
	prior := identity()
	prior["ttl"] = num(300)
	plan := identity()
	plan["ttl"] = num(600)

	require.Error(t, r.update(context.Background(), prior, plan))
	assert.Equal(t, 1, calls[spec.Update])
}
