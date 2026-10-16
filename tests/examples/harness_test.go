//go:build integration

// Package examples runs Terraform configs from testdata/ against one
// technitium/dns-server container that TestMain starts and removes.
package examples

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
	"github.com/ilijamt/terraform-provider-technitium/internal/provider"
	"github.com/ilijamt/terraform-provider-technitium/internal/technitium"
	"github.com/ilijamt/terraform-provider-technitium/internal/testserver"
)

// host and token reach the container TestMain started.
var host, token string

// server is the container TestMain started, for its ID and IP.
var server *testserver.Container

// envVars may point at a real server. The provider falls back to them for a
// value its block leaves out, so they are cleared before any test runs.
var envVars = []string{"TECHNITIUM_HOST", "TECHNITIUM_TOKEN", "TECHNITIUM_USERNAME", "TECHNITIUM_PASSWORD", "TECHNITIUM_INSECURE"}

func TestMain(m *testing.M) {
	for _, k := range envVars {
		_ = os.Unsetenv(k)
	}
	if os.Getenv("TF_ACC") == "" {
		fmt.Fprintln(os.Stderr, "set TF_ACC=1 to run the integration tests")
		os.Exit(m.Run())
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var err error
	if server, err = testserver.Launch(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	host = server.Host
	stop := server.Stop
	// os.Exit skips deferred calls, so an interrupted run would leak the container.
	go func() {
		<-ctx.Done()
		stop()
		os.Exit(1)
	}()
	if token, err = createToken(host); err != nil {
		stop()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// OpenTofu resolves "ilijamt/technitium" to registry.opentofu.org and
	// rejects the harness's default registry.terraform.io/-/technitium.
	if os.Getenv("TF_ACC_PROVIDER_HOST") == "" && openTofu(os.Getenv("TF_ACC_TERRAFORM_PATH")) {
		_ = os.Setenv("TF_ACC_PROVIDER_HOST", "registry.opentofu.org")
	}
	code := m.Run()
	stop()
	os.Exit(code)
}

// createToken makes an API token for the admin user. The token answers at
// the top level of the reply, where client.Call does not look.
func createToken(host string) (string, error) {
	res, err := http.PostForm(host+"/api/user/createToken", url.Values{
		"user": {testserver.Username}, "pass": {testserver.Password}, "tokenName": {"integration"},
	})
	if err != nil {
		return "", fmt.Errorf("create token: %w", err)
	}
	defer res.Body.Close()
	var out struct {
		Status       string `json:"status"`
		ErrorMessage string `json:"errorMessage"`
		Token        string `json:"token"`
	}
	if err = json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("create token: %w", err)
	}
	if out.Status != "ok" || out.Token == "" {
		return "", fmt.Errorf("create token: %s: %s", out.Status, out.ErrorMessage)
	}
	return out.Token, nil
}

func openTofu(path string) bool {
	if path == "" {
		return false
	}
	out, err := exec.Command(path, "version").Output()
	return err == nil && bytes.HasPrefix(out, []byte("OpenTofu"))
}

var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"technitium": providerserver.NewProtocol6WithError(provider.New("test", nil, technitium.Resources(), technitium.DataSources())()),
}

// providerHeader sets every connection value, so the provider never falls
// back to the environment.
func providerHeader() string {
	return fmt.Sprintf(`
provider "technitium" {
  host  = %q
  token = %q
}
`, host, token)
}

// FixturePath resolves testdata/<name>/<file> next to this source file.
func FixturePath(t *testing.T, name, file string) string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve the testdata directory")
	}
	return filepath.Join(filepath.Dir(self), "testdata", name, file)
}

// ReadFixture returns testdata/<name>/<file> behind the provider block.
func ReadFixture(t *testing.T, name, file string) string {
	t.Helper()
	b, err := os.ReadFile(FixturePath(t, name, file))
	require.NoError(t, err)
	return providerHeader() + string(b)
}

// api is a client for the out-of-band calls a test makes to set up drift or
// check what the server holds.
func api() *client.Client {
	return client.New(host, token, "test", false, nil)
}

func call(t *testing.T, path string, params url.Values) map[string]any {
	t.Helper()
	resp, err := api().Call(context.Background(), path, params)
	require.NoError(t, err, path)
	return resp
}

// records returns the records of type typ at domain.
func records(t *testing.T, zone, domain, typ string) []map[string]any {
	t.Helper()
	resp := call(t, "/api/zones/records/get", url.Values{"zone": {zone}, "domain": {domain}})
	var out []map[string]any
	for _, r := range resp["records"].([]any) {
		if rec := r.(map[string]any); rec["type"] == typ {
			out = append(out, rec)
		}
	}
	return out
}

func zonesDestroyed(zones ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		for _, zone := range zones {
			_, err := api().Call(context.Background(), "/api/zones/options/get", url.Values{"zone": {zone}})
			if err == nil {
				return fmt.Errorf("zone %s still exists", zone)
			}
			if !client.IsNotFound(err) {
				return err
			}
		}
		return nil
	}
}
