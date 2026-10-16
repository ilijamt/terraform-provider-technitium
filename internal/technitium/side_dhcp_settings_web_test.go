package technitium_test

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
	"github.com/ilijamt/terraform-provider-technitium/internal/testserver"
)

// ownTestServer runs a server of its own for checks that restart services.
func ownTestServer(t *testing.T) (string, *client.Client) {
	host, _, c := ownTestServerWith(t)
	return host, c
}

// ownTestServerWith also publishes ports on 127.0.0.1, and returns the
// container's IP.
func ownTestServerWith(t *testing.T, ports ...int) (host, ip string, c *client.Client) {
	t.Helper()
	acceptance(t)
	if !ownServer {
		t.Skip("needs a test server in Docker; TECHNITIUM_TESTACC_HOST is set")
	}
	host, ip, stop, err := testserver.Run(context.Background(), ports...)
	require.NoError(t, err)
	t.Cleanup(stop)
	c, err = loginTo(host)
	require.NoError(t, err)
	return host, ip, c
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
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

// TFTECH-126: a new port moves the service away from the provider's host.
// The settings change applies, later calls in that apply fail at once
// naming the new address, and with the host set to it the plan is clean.
func TestAccSideSettingsWebServicePortMoves(t *testing.T) {
	port := freePort(t)
	host, _, _ := ownTestServerWith(t, port)
	moved := fmt.Sprintf("http://127.0.0.1:%d", port)
	settings := fmt.Sprintf(`
resource "technitium_settings_web_service" "this" {
  web_service_http_port = %d
}
`, port)
	var start time.Time
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			PreConfig: func() { start = time.Now() },
			Config: providerFor(host) + settings + `
resource "technitium_zone" "after" {
  zone       = "moved.example"
  type       = "Primary"
  depends_on = [technitium_settings_web_service.this]
}
`,
			ExpectError: regexp.MustCompile(strings.ReplaceAll(regexp.QuoteMeta("the web service moved to "+moved+" after a settings change; set the provider host to it and apply again"), " ", `\s+`)),
		}, {
			PreConfig: func() { require.Less(t, time.Since(start), 30*time.Second, "the move is not waited out") },
			Config:    providerFor(moved) + settings,
			PlanOnly:  true,
		}},
	})
}

// TFTECH-126: local addresses without the host's address move the service.
// One that still serves the host, as the container's address does behind
// Docker's port forwarding, does not stop the apply.
func TestAccSideSettingsWebServiceAddressMoves(t *testing.T) {
	host, ip, _ := ownTestServerWith(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerFor(host) + fmt.Sprintf(`
resource "technitium_settings_web_service" "this" {
  web_service_local_addresses = [%q]
}

resource "technitium_zone" "after" {
  zone       = "still.example"
  type       = "Primary"
  depends_on = [technitium_settings_web_service.this]
}
`, ip),
		}},
	})

	// Terraform could not destroy what a moving step created, so the move
	// is checked on the client.
	c, err := loginTo(host)
	require.NoError(t, err)
	start := time.Now()
	_, err = c.Call(client.Restarts(context.Background()), "/api/settings/set", url.Values{"webServiceLocalAddresses": {"127.0.0.2"}, "webServiceHttpPort": {"5380"}})
	require.NoError(t, err)
	_, err = c.Call(context.Background(), "/api/settings/get", nil)
	require.ErrorContains(t, err, "the web service moved to http://127.0.0.2:5380 after a settings change")
	require.Less(t, time.Since(start), 15*time.Second)
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

// TFTECH-126: with TLS off the HTTPS redirect sends every request to a port
// nothing listens on, so the provider refuses it at plan time.
func TestAccSideSettingsWebServiceRedirectWithoutTLS(t *testing.T) {
	host, c := ownTestServer(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerFor(host) + `
resource "technitium_settings_web_service" "this" {
  web_service_enable_tls           = false
  web_service_http_to_tls_redirect = true
}
`,
			ExpectError: regexp.MustCompile(`web_service_http_to_tls_redirect needs web_service_enable_tls = true`),
		}},
	})
	require.Equal(t, false, call(t, c, "/api/settings/get", nil)["webServiceHttpToTlsRedirect"])
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
