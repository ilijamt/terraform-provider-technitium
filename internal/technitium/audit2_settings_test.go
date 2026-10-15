package technitium_test

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/testserver"
)

func serverConfig(t *testing.T) []byte {
	t.Helper()
	out, err := exec.Command("docker", "ps", "--filter", "label="+testserver.Label, "--format", "{{.ID}} {{.Ports}}").Output()
	require.NoError(t, err)
	addr := strings.TrimPrefix(os.Getenv("TECHNITIUM_HOST"), "http://")
	for line := range strings.Lines(string(out)) {
		id, ports, _ := strings.Cut(strings.TrimSpace(line), " ")
		if strings.Contains(ports, addr+"->") {
			cfg, err := exec.Command("docker", "exec", id, "cat", "/etc/dns/dns.config").Output()
			require.NoError(t, err)
			return cfg
		}
	}
	t.Fatalf("no container publishes %s", addr)
	return nil
}

// passwordKept waits for the server to save forwarder, as it saves settings
// a few seconds after a change, and checks password was saved with it.
func passwordKept(t *testing.T, forwarder, password string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		cfg := serverConfig(t)
		for i := 0; i < 30 && !bytes.Contains(cfg, []byte(forwarder)); i++ {
			time.Sleep(time.Second)
			cfg = serverConfig(t)
		}
		if !bytes.Contains(cfg, []byte(forwarder)) {
			return fmt.Errorf("the server did not save forwarder %s", forwarder)
		}
		if !bytes.Contains(cfg, []byte(password)) {
			return fmt.Errorf("the server no longer stores the proxy password %q", password)
		}
		return nil
	}
}

func TestAccSettingsProxyPasswordKept(t *testing.T) {
	acceptance(t)
	if !ownServer {
		t.Skip("reads the server's files in its container")
	}
	c := api(t)
	t.Cleanup(func() { call(t, c, "/api/settings/set", url.Values{"proxyType": {"None"}, "forwarders": {"false"}}) })

	call(t, c, "/api/settings/set", url.Values{
		"proxyType": {"Socks5"}, "proxyAddress": {"127.0.0.1"}, "proxyPort": {"3128"},
		"proxyUsername": {"proxyuser"}, "proxyPassword": {"OutsidePass53"}, "forwarders": {"8.8.4.4"},
	})
	require.NoError(t, passwordKept(t, "8.8.4.4", "OutsidePass53")(nil), "the check must see the password")
	forwarders := func(f string) string {
		return `resource "technitium_settings_proxy_forwarders" "this" { forwarders = ["` + f + `"] }`
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: forwarders("1.1.1.1"), Check: passwordKept(t, "1.1.1.1", "OutsidePass53")},
			{Config: forwarders("9.9.9.9"), Check: passwordKept(t, "9.9.9.9", "OutsidePass53")},
		},
	})

	config := func(f, password string) string {
		return fmt.Sprintf(`
resource "technitium_settings_proxy_forwarders" "this" {
  proxy_type     = "Socks5"
  proxy_address  = "127.0.0.1"
  proxy_port     = 3128
  proxy_username = "proxyuser"
  proxy_password = %q
  forwarders     = [%q]
}
`, password, f)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: config("1.1.1.1", "ConfiguredPass53"), Check: passwordKept(t, "1.1.1.1", "ConfiguredPass53")},
			{Config: config("9.9.9.9", "ConfiguredPass53"), Check: passwordKept(t, "9.9.9.9", "ConfiguredPass53")},
			{Config: config("1.0.0.1", "ChangedPass53"), Check: passwordKept(t, "1.0.0.1", "ChangedPass53")},
		},
	})
}
