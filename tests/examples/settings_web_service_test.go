//go:build integration

package examples

import (
	"net/url"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/require"
)

// TFTECH-112: the server turns TLS off when it has no certificate, so the
// provider refuses that at plan time.
func TestIntegration_SideSettingsWebServiceTLSWithoutCertificate(t *testing.T) {
	host, c := ownTestServer(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      ReadFixtureFor(t, host, "side_settings_web_service_tls_without_certificate", "main.tf"),
			ExpectError: regexp.MustCompile(`web_service_enable_tls needs web_service_tls_certificate_path`),
		}},
	})
	resp := callWith(t, c, "/api/settings/set", url.Values{"webServiceEnableTls": {"true"}, "webServiceUseSelfSignedTlsCertificate": {"false"}, "webServiceTlsCertificatePath": {""}})
	require.Equal(t, false, resp["webServiceEnableTls"])
}

// TFTECH-126: with TLS off the HTTPS redirect sends every request to a port
// nothing listens on, so the provider refuses it at plan time.
func TestIntegration_SideSettingsWebServiceRedirectWithoutTLS(t *testing.T) {
	host, c := ownTestServer(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      ReadFixtureFor(t, host, "side_settings_web_service_redirect_without_tls", "main.tf"),
			ExpectError: regexp.MustCompile(`web_service_http_to_tls_redirect needs web_service_enable_tls = true`),
		}},
	})
	require.Equal(t, false, callWith(t, c, "/api/settings/get", nil)["webServiceHttpToTlsRedirect"])
}

// TFTECH-112, TFTECH-123: the server stores an empty end point list as the
// defaults and port 0 or none as 53, so the plan refuses them.
func TestIntegration_SideSettingsDNSEndPointDefaults(t *testing.T) {
	host, c := ownTestServer(t)
	before := callWith(t, c, "/api/settings/get", nil)["dnsServerLocalEndPoints"]
	for file, want := range map[string]string{
		"port_zero.tf": `"0.0.0.0:0" needs a port`,
		"no_port.tf":   `"0.0.0.0" needs a port`,
		"empty.tf":     `an empty list`,
	} {
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: factories,
			Steps: []resource.TestStep{{
				Config:      ReadFixtureFor(t, host, "side_settings_dns_end_point_defaults", file),
				ExpectError: regexp.MustCompile(want),
			}},
		})
		require.Equal(t, before, callWith(t, c, "/api/settings/get", nil)["dnsServerLocalEndPoints"], file)
	}
}
