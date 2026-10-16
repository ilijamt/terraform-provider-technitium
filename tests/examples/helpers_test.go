//go:build integration

package examples

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
	"github.com/ilijamt/terraform-provider-technitium/internal/testserver"
)

// acceptance is for tests that call the server before resource.Test, which
// skips on its own. Without TF_ACC, TestMain starts no server.
func acceptance(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run the integration tests")
	}
}

// skipBelow skips a test that needs a newer Terraform or OpenTofu than the
// binary the harness runs.
func skipBelow(t *testing.T, terraform, tofu, why string) {
	path := os.Getenv("TF_ACC_TERRAFORM_PATH")
	if path == "" {
		var err error
		if path, err = exec.LookPath("terraform"); err != nil {
			return
		}
	}
	out, err := exec.Command(path, "version", "-json").Output()
	require.NoError(t, err)
	var v struct {
		Version string `json:"terraform_version"`
	}
	require.NoError(t, json.Unmarshal(out, &v))
	have, err := version.NewVersion(v.Version)
	require.NoError(t, err)
	name, minimum := "Terraform", terraform
	if openTofu(path) {
		name, minimum = "OpenTofu", tofu
	}
	if have.LessThan(version.Must(version.NewVersion(minimum))) {
		t.Skipf("needs %s %s or later for %s, have %s", name, minimum, why, have)
	}
}

// callWith is call against another server, or as another user.
func callWith(t *testing.T, c *client.Client, path string, params url.Values) map[string]any {
	t.Helper()
	resp, err := c.Call(context.Background(), path, params)
	require.NoError(t, err, path)
	return resp
}

// loginTo signs in to a server of a test's own as the admin user.
func loginTo(host string) (*client.Client, error) {
	c := client.New(host, "", "test", false, nil)
	return c, c.Login(context.Background(), testserver.Username, testserver.Password)
}

func merge(a, b url.Values) url.Values {
	out := url.Values{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// names returns the strings in list, or the key of each object in it.
func names(list any, key string) []string {
	var out []string
	items, _ := list.([]any)
	for _, i := range items {
		switch v := i.(type) {
		case string:
			out = append(out, v)
		case map[string]any:
			s, _ := v[key].(string)
			out = append(out, s)
		}
	}
	return out
}

func expect(addr string, action plancheck.ResourceActionType) resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, action)}}
}

func expectCreate(addr string) resource.ConfigPlanChecks {
	return expect(addr, plancheck.ResourceActionCreate)
}

func expectUpdate(addr string) resource.ConfigPlanChecks {
	return expect(addr, plancheck.ResourceActionUpdate)
}

// expectReplace expects destroy then create, the only replace the provider
// plans.
func expectReplace(addr string) resource.ConfigPlanChecks {
	return expect(addr, plancheck.ResourceActionDestroyBeforeCreate)
}

func expectNoop(addr string) resource.ConfigPlanChecks {
	return expect(addr, plancheck.ResourceActionNoop)
}

// resetSSO turns SSO off when the test ends.
func resetSSO(t *testing.T) {
	t.Cleanup(func() {
		_, _ = api().Call(context.Background(), "/api/admin/sso/set", url.Values{
			"ssoEnabled": {"false"}, "ssoAuthority": {""}, "ssoClientId": {""}, "ssoClientSecret": {""},
			"ssoAllowSignup": {"false"}, "ssoGroupMap": {""},
		})
		// The change restarts the web service a moment later; let it
		// finish before the next test calls the server.
		time.Sleep(2 * time.Second)
		_, _ = api().Call(context.Background(), "/api/user/session/get", nil)
	})
}

// resetLDAP turns LDAP off when the test ends.
func resetLDAP(t *testing.T) {
	t.Cleanup(func() {
		_, _ = api().Call(context.Background(), "/api/admin/ldap/set", url.Values{
			"ldapEnabled": {"false"}, "ldapServer": {""}, "ldapBindUsername": {""}, "ldapBindPassword": {""},
			"ldapSearchBase": {""}, "ldapUserSearchFilter": {""}, "ldapGroupAttribute": {""}, "ldapGroupMap": {""}, "ldapAllowSignup": {"false"},
		})
	})
}

// storeURL returns the URL of an app in the DNS App Store, and skips the
// test when the store is unreachable.
func storeURL(t *testing.T, name string) string {
	resp, err := api().Call(context.Background(), "/api/apps/listStoreApps", nil)
	if err != nil {
		t.Skipf("DNS App Store unreachable: %s", err)
	}
	return appURL(t, resp, name)
}

// tryStoreURL is storeURL that returns "" when the store is unreachable.
func tryStoreURL(t *testing.T, name string) string {
	resp, err := api().Call(context.Background(), "/api/apps/listStoreApps", nil)
	if err != nil {
		return ""
	}
	return appURL(t, resp, name)
}

func appURL(t *testing.T, resp map[string]any, name string) string {
	for _, a := range resp["storeApps"].([]any) {
		if app := a.(map[string]any); app["name"] == name {
			u, _ := app["url"].(string)
			return u
		}
	}
	t.Fatalf("%s is not in the DNS App Store", name)
	return ""
}

// readConfig joins the .tf files in dir, such as an export's output. It adds
// no provider block; put providerHeader() in front to apply it.
func readConfig(t *testing.T, dir string) string {
	files, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	require.NoError(t, err)
	var b strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(f)
		require.NoError(t, err)
		b.Write(data)
		b.WriteString("\n")
	}
	return b.String()
}

// ownContainer runs a server of the test's own, removed when the test ends,
// with each of ports also published on the same port of 127.0.0.1.
func ownContainer(t *testing.T, ports ...int) (*testserver.Container, *client.Client) {
	t.Helper()
	acceptance(t)
	s, err := testserver.Launch(context.Background(), ports...)
	require.NoError(t, err)
	t.Cleanup(s.Stop)
	c, err := loginTo(s.Host)
	require.NoError(t, err)
	return s, c
}

// ownTestServer runs a server of the test's own, for checks that restart
// services or need a second server.
func ownTestServer(t *testing.T) (string, *client.Client) {
	s, c := ownContainer(t)
	return s.Host, c
}

// ownTestServerWith also publishes ports, and returns the container's IP on
// the Docker network, where the shared server reaches it.
func ownTestServerWith(t *testing.T, ports ...int) (host, ip string, c *client.Client) {
	s, c := ownContainer(t, ports...)
	return s.Host, s.IP, c
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// providerFor is the provider block for a server of the test's own.
func providerFor(host string) string {
	return fmt.Sprintf(`
provider "technitium" {
  host     = %q
  username = %q
  password = %q
}
`, host, testserver.Username, testserver.Password)
}

// ReadFixtureFor is ReadFixture for a server of the test's own.
func ReadFixtureFor(t *testing.T, host, name, file string) string {
	t.Helper()
	b, err := os.ReadFile(FixturePath(t, name, file))
	require.NoError(t, err)
	return providerFor(host) + string(b)
}

// dockerExec runs a command in the shared server's container.
func dockerExec(t *testing.T, args ...string) []byte {
	t.Helper()
	return dockerExecIn(t, server.ID, args...)
}

// dockerExecIn runs a command in container id, such as an ownContainer's.
func dockerExecIn(t *testing.T, id string, args ...string) []byte {
	t.Helper()
	out, err := exec.Command("docker", append([]string{"exec", id}, args...)...).Output()
	require.NoError(t, err, "docker exec %v", args)
	return out
}
