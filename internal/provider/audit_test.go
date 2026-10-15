package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureHost(t *testing.T) {
	for _, host := range []string{"dns:5380", "localhost:5380", "ftp://dns:5380", "http://", "http//dns", "://dns"} {
		t.Run(host, func(t *testing.T) {
			clearEnv(t)
			resp := configure(t, map[string]tftypes.Value{"host": str(host), "token": str("t0k")})
			require.True(t, resp.Diagnostics.HasError(), host)
			d := resp.Diagnostics.Errors()[0]
			assert.Equal(t, "Invalid host", d.Summary())
			wp, ok := d.(diag.DiagnosticWithPath)
			require.True(t, ok)
			assert.Equal(t, path.Root("host"), wp.Path())
		})
	}
	for _, host := range []string{"http://dns:5380", "https://dns.example/", "HTTPS://dns:53443/base"} {
		clearEnv(t)
		resp := configure(t, map[string]tftypes.Value{"host": str(host), "token": str("t0k")})
		assert.False(t, resp.Diagnostics.HasError(), "%s: %v", host, resp.Diagnostics)
	}
}
