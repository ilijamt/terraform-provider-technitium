package technitium

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
)

func TestCheckLocalEndPoints(t *testing.T) {
	list := tftypes.List{ElementType: tftypes.String}
	check := func(v tftypes.Value) error {
		return checkLocalEndPoints(nil, map[string]tftypes.Value{"dns_server_local_end_points": v})
	}
	eps := func(s ...string) tftypes.Value {
		var out []tftypes.Value
		for _, e := range s {
			out = append(out, tftypes.NewValue(tftypes.String, e))
		}
		return tftypes.NewValue(list, out)
	}

	assert.NoError(t, check(tftypes.NewValue(list, nil)), "unset")
	assert.NoError(t, check(tftypes.NewValue(list, tftypes.UnknownValue)), "unknown")
	assert.NoError(t, check(eps("0.0.0.0:53", "[::]:5353")))
	assert.NoError(t, check(tftypes.NewValue(list, []tftypes.Value{tftypes.NewValue(tftypes.String, tftypes.UnknownValue)})), "unknown end point")
	assert.ErrorContains(t, check(tftypes.NewValue(list, []tftypes.Value{})), "empty list")
	assert.ErrorContains(t, check(eps("0.0.0.0:53", "[::]:0")), `"[::]:0" needs a port`)
	assert.ErrorContains(t, check(eps("0.0.0.0")), `"0.0.0.0" needs a port`)
}

// TFTECH-126: the values whose change restarts the web service, as
// TestAccSideSettingsRestarts finds. The ones that restart only the DNS
// service are left out: the HTTP API stays up through that.
func TestSettingsRestarts(t *testing.T) {
	got := map[string][]string{}
	for _, s := range append(settings, settingsSSO, settingsLDAP) {
		for _, a := range s.Attrs {
			if a.Restarts {
				got[s.Name] = append(got[s.Name], a.Name)
			}
		}
	}
	assert.Equal(t, map[string][]string{
		"settings_web_service": {
			"web_service_local_addresses", "web_service_http_port",
			"web_service_enable_http_unix_socket", "web_service_http_unix_socket",
			"web_service_enable_tls_unix_socket", "web_service_tls_unix_socket",
			"web_service_enable_tls", "web_service_enable_http3", "web_service_http_to_tls_redirect", "web_service_tls_port",
			// They restart it only when they leave TLS on without a certificate.
			"web_service_use_self_signed_tls_certificate", "web_service_tls_certificate_path",
		},
		// They restart it unless SSO is and stays disabled.
		"settings_sso": {"enabled", "authority", "client_id", "client_secret", "metadata_address", "scopes"},
	}, got)
}
