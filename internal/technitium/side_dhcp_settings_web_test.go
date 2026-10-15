package technitium_test

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
	"github.com/ilijamt/terraform-provider-technitium/internal/testserver"
)

// ownTestServer runs a server of its own for checks that restart services.
func ownTestServer(t *testing.T) (string, *client.Client) {
	t.Helper()
	acceptance(t)
	if !ownServer {
		t.Skip("needs a test server in Docker; TECHNITIUM_TESTACC_HOST is set")
	}
	host, _, stop, err := testserver.Run(context.Background())
	require.NoError(t, err)
	t.Cleanup(stop)
	c, err := loginTo(host)
	require.NoError(t, err)
	return host, c
}

func providerFor(host string) string {
	return fmt.Sprintf(`
provider "technitium" {
  host     = %q
  username = %q
  password = %q
}
`, host, testserver.Username, testserver.Password)
}

// TFTECH-112: the web service restarts on its new port about 2s after the
// settings call returns, so later calls to the old port fail.
func TestAccSideSettingsWebServicePortRestarts(t *testing.T) {
	_, c := ownTestServer(t)
	call(t, c, "/api/settings/set", url.Values{"webServiceHttpPort": {"5381"}})
	_, err := c.Call(context.Background(), "/api/settings/get", nil)
	require.NoError(t, err, "the service answers until it restarts")
	time.Sleep(4 * time.Second)
	_, err = c.Call(context.Background(), "/api/settings/get", nil)
	require.Error(t, err, "the old port no longer answers")
}

// TFTECH-112: the server turns TLS off when it has no certificate, so the
// provider refuses that at plan time.
func TestAccSideSettingsWebServiceTLSWithoutCertificate(t *testing.T) {
	host, c := ownTestServer(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerFor(host) + `
resource "technitium_settings_web_service" "this" {
  web_service_enable_tls                      = true
  web_service_use_self_signed_tls_certificate = false
}
`,
			ExpectError: regexp.MustCompile(`web_service_enable_tls needs web_service_tls_certificate_path`),
		}},
	})
	resp := call(t, c, "/api/settings/set", url.Values{"webServiceEnableTls": {"true"}, "webServiceUseSelfSignedTlsCertificate": {"false"}, "webServiceTlsCertificatePath": {""}})
	require.Equal(t, false, resp["webServiceEnableTls"])
}

// TFTECH-112, TFTECH-123: the server stores an empty end point list as the
// defaults and port 0 or none as 53, so the plan refuses them.
func TestAccSideSettingsDNSEndPointDefaults(t *testing.T) {
	host, c := ownTestServer(t)
	before := call(t, c, "/api/settings/get", nil)["dnsServerLocalEndPoints"]
	for endPoints, want := range map[string]string{
		`["0.0.0.0:0"]`: `"0.0.0.0:0" needs a port`,
		`["0.0.0.0"]`:   `"0.0.0.0" needs a port`,
		`[]`:            `an empty list`,
	} {
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: factories,
			Steps: []resource.TestStep{{
				Config: providerFor(host) + `
resource "technitium_settings_general" "this" {
  dns_server_local_end_points = ` + endPoints + `
}
`,
				ExpectError: regexp.MustCompile(want),
			}},
		})
		require.Equal(t, before, call(t, c, "/api/settings/get", nil)["dnsServerLocalEndPoints"], endPoints)
	}
}
