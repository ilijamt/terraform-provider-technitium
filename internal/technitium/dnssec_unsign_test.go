package technitium

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
)

// TFTECH-124: an unsign cut off mid-way is not sent again, so the status
// read afterwards decides whether it took effect.
func TestDNSSECDeleteDroppedUnsign(t *testing.T) {
	ctx := context.Background()
	r := &zoneDNSSEC{}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	typ := s.Schema.Type().TerraformType(ctx).(tftypes.Object)
	vals := map[string]tftypes.Value{}
	for name, at := range typ.AttributeTypes {
		vals[name] = tftypes.NewValue(at, nil)
	}
	vals["zone"] = tftypes.NewValue(tftypes.String, "example.test")
	state := tfsdk.State{Schema: s.Schema, Raw: tftypes.NewValue(typ, vals)}

	for status, ok := range map[string]bool{"Unsigned": true, "SignedWithNSEC": false} {
		t.Run(status, func(t *testing.T) {
			calls := map[string]int{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls[req.URL.Path]++
				if req.URL.Path == "/api/zones/dnssec/unsign" {
					conn, _, err := w.(http.Hijacker).Hijack()
					require.NoError(t, err)
					_ = conn.Close()
					return
				}
				_, _ = w.Write([]byte(`{"status":"ok","response":{"dnssecStatus":"` + status + `"}}`))
			}))
			defer srv.Close()
			r.api = client.New(srv.URL, "t0k", "test", false, nil)

			var resp resource.DeleteResponse
			r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
			assert.Equal(t, 1, calls["/api/zones/dnssec/unsign"], "unsign is not sent again")
			assert.Equal(t, 1, calls["/api/zones/dnssec/properties/get"])
			assert.Equal(t, !ok, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
		})
	}
}
