//go:build integration

package examples

import (
	"bytes"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"
)

// proxyPasswordKept waits for the server to save forwarder, as it saves
// settings a few seconds after a change, and checks password was saved with it.
func proxyPasswordKept(t *testing.T, forwarder, password string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		cfg := dockerExec(t, "cat", "/etc/dns/dns.config")
		for i := 0; i < 30 && !bytes.Contains(cfg, []byte(forwarder)); i++ {
			time.Sleep(time.Second)
			cfg = dockerExec(t, "cat", "/etc/dns/dns.config")
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

func TestIntegration_SettingsProxyPasswordKept(t *testing.T) {
	acceptance(t)
	const name = "settings_proxy_password_kept"
	t.Cleanup(func() { call(t, "/api/settings/set", url.Values{"proxyType": {"None"}, "forwarders": {"false"}}) })

	call(t, "/api/settings/set", url.Values{
		"proxyType": {"Socks5"}, "proxyAddress": {"127.0.0.1"}, "proxyPort": {"3128"},
		"proxyUsername": {"proxyuser"}, "proxyPassword": {"OutsidePass53"}, "forwarders": {"8.8.4.4"},
	})
	require.NoError(t, proxyPasswordKept(t, "8.8.4.4", "OutsidePass53")(nil), "the check must see the password")
	forwarders := func(f string) resource.TestStep {
		return resource.TestStep{
			Config:          ReadFixture(t, name, "forwarders.tf"),
			ConfigVariables: config.Variables{"forwarder": config.StringVariable(f)},
			Check:           proxyPasswordKept(t, f, "OutsidePass53"),
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps:                    []resource.TestStep{forwarders("1.1.1.1"), forwarders("9.9.9.9")},
	})

	proxy := func(f, password string) resource.TestStep {
		return resource.TestStep{
			Config:          ReadFixture(t, name, "proxy.tf"),
			ConfigVariables: config.Variables{"forwarder": config.StringVariable(f), "password": config.StringVariable(password)},
			Check:           proxyPasswordKept(t, f, password),
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			proxy("1.1.1.1", "ConfiguredPass53"),
			proxy("9.9.9.9", "ConfiguredPass53"),
			proxy("1.0.0.1", "ChangedPass53"),
		},
	})
}
