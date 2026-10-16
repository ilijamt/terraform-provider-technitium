package technitium_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/testserver"
)

// TFTECH-126: pins which settings changes make the server restart its web or
// DNS service. The server logs each restart, which is what this checks; the
// outage the restart causes is too short to catch every time.
func TestAccSideSettingsRestarts(t *testing.T) {
	host, _ := ownTestServer(t)
	s := &restartServer{host: host, http: &http.Client{Timeout: 5 * time.Second}}
	s.login(t)

	sso := "/api/admin/sso/set"
	set := "/api/settings/set"
	for _, step := range []struct {
		name     string
		path     string
		params   url.Values
		web, dns bool
	}{
		{"web addresses", set, url.Values{"webServiceLocalAddresses": {"0.0.0.0"}}, true, false},
		{"web addresses unchanged", set, url.Values{"webServiceLocalAddresses": {"0.0.0.0"}}, false, false},
		{"enable tls", set, url.Values{"webServiceEnableTls": {"true"}, "webServiceUseSelfSignedTlsCertificate": {"true"}}, true, false},
		{"tls loses its certificate", set, url.Values{"webServiceUseSelfSignedTlsCertificate": {"false"}}, true, false},
		{"http unix socket", set, url.Values{"webServiceEnableHttpUnixSocket": {"true"}}, true, false},
		{"real ip header", set, url.Values{"webServiceRealIpHeader": {"X-Forwarded-For"}}, false, false},
		{"tcp send timeout", set, url.Values{"tcpSendTimeout": {"20000"}}, false, true},
		{"tcp send timeout unchanged", set, url.Values{"tcpSendTimeout": {"20000"}}, false, false},
		{"tcp proxy", set, url.Values{"enableDnsOverTcpProxy": {"true"}}, false, true},
		{"dns end points", set, url.Values{"dnsServerLocalEndPoints": {"0.0.0.0:53"}}, false, true},
		{"server domain", set, url.Values{"dnsServerDomain": {"restart.test"}}, false, false},
		{"client timeout", set, url.Values{"clientTimeout": {"3000"}}, false, false},
		{"sso stays disabled", sso, url.Values{"ssoEnabled": {"false"}, "ssoAuthority": {"https://idp.example.com/a"}}, false, false},
		{"sso disabled without ssoEnabled", sso, url.Values{"ssoAuthority": {"https://idp.example.com/b"}}, true, false},
		{"sso enabled", sso, url.Values{"ssoEnabled": {"true"}}, true, false},
		{"sso client id", sso, url.Values{"ssoClientId": {"technitium"}}, true, false},
		{"sso allow signup", sso, url.Values{"ssoAllowSignup": {"true"}}, false, false},
		{"sso disabled", sso, url.Values{"ssoEnabled": {"false"}}, true, false},
		{"ldap", "/api/admin/ldap/set", url.Values{"ldapServer": {"ldap.example.com"}}, false, false},
	} {
		before := s.restarts(t)
		probe := s.probe()
		s.call(t, step.path, step.params)
		want := before
		if step.web {
			want[0]++
		}
		if step.dns {
			want[1]++
		}
		// The web service restarts 2s after the reply, so a restart that has
		// not come after 6s is not coming.
		got := before
		for deadline := time.Now().Add(6 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
			if got = s.restarts(t); got != before && got == want {
				break
			}
		}
		if got == want && (step.web || step.dns) {
			// Let the restart finish and see it in the probe.
			time.Sleep(time.Second)
		}
		down := probe()
		t.Logf("%s: restarts web %d dns %d; down %s", step.name, got[0]-before[0], got[1]-before[1], down)
		require.Equal(t, want, got, "%s: web and DNS service restarts", step.name)
	}
}

// TFTECH-126: a settings change that restarts the web service drops the calls
// in flight 2s after it returns, so the provider holds them until it is back.
// The records added after each change keep calls flowing through the restart,
// and an add cut off is not sent again. The outage lasts a few ms, so the test
// restarts three times to hit it more often.
func TestAccSideSettingsRestartHoldsCalls(t *testing.T) {
	host, _ := ownTestServer(t)
	config := func(clientID string, records int) string {
		return providerFor(host) + fmt.Sprintf(`
resource "technitium_settings_sso" "this" {
  enabled   = true
  authority = "https://idp.example.com/realms/home"
  client_id = %q
}

resource "technitium_zone" "test" {
  count = 8
  zone  = "restart${count.index}.test"
}

resource "technitium_record_a" "test" {
  count      = %d
  zone       = technitium_zone.test[count.index %% 8].zone
  domain     = "r${count.index}.${technitium_zone.test[count.index %% 8].zone}"
  ip_address = "10.0.0.1"
  depends_on = [technitium_settings_sso.this]
}
`, clientID, records)
	}
	steps := []resource.TestStep{{Config: config("c0", 0)}}
	for i := 1; i <= 3; i++ {
		id, records := fmt.Sprintf("c%d", i), 300*i
		steps = append(steps, resource.TestStep{
			Config: config(id, records),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("technitium_settings_sso.this", "client_id", id),
				resource.TestCheckResourceAttr(fmt.Sprintf("technitium_record_a.test.%d", records-1), "ip_address", "10.0.0.1"),
			),
		})
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: steps})

	s := &restartServer{host: host, http: &http.Client{Timeout: 5 * time.Second}}
	s.login(t)
	require.GreaterOrEqual(t, s.restarts(t)[0], 4, "the web service restarted on each change")
}

type restartServer struct {
	host  string
	token string
	http  *http.Client
}

func (s *restartServer) login(t *testing.T) {
	resp := s.call(t, "/api/user/login", url.Values{"user": {testserver.Username}, "pass": {testserver.Password}})
	s.token, _ = resp["token"].(string)
	require.NotEmpty(t, s.token)
}

// call retries while the web service is restarting from the previous step.
func (s *restartServer) call(t *testing.T, path string, params url.Values) map[string]any {
	t.Helper()
	var err error
	for range 40 {
		var body []byte
		if body, err = s.post(path, params); err == nil {
			var out map[string]any
			require.NoError(t, json.Unmarshal(body, &out), path)
			require.Equal(t, "ok", out["status"], "%s: %s", path, out["errorMessage"])
			return out
		}
		time.Sleep(250 * time.Millisecond)
	}
	require.NoError(t, err, path)
	return nil
}

func (s *restartServer) post(path string, params url.Values) ([]byte, error) {
	params = maps.Clone(params)
	if params == nil {
		params = url.Values{}
	}
	if s.token != "" {
		params.Set("token", s.token)
	}
	resp, err := s.http.PostForm(s.host+path, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err == nil && resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("%s: %s", path, resp.Status)
	}
	return body, err
}

// restarts counts the web and DNS service restarts in the server's log.
func (s *restartServer) restarts(t *testing.T) [2]int {
	t.Helper()
	files, _ := s.call(t, "/api/logs/list", nil)["response"].(map[string]any)["logFiles"].([]any)
	var n [2]int
	for _, f := range files {
		name, _ := f.(map[string]any)["fileName"].(string)
		var body []byte
		var err error
		for range 40 {
			if body, err = s.post("/api/logs/download", url.Values{"fileName": {name}}); err == nil {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		require.NoError(t, err)
		n[0] += strings.Count(string(body), "Attempting to restart web service.")
		n[1] += strings.Count(string(body), "Attempting to restart DNS service.")
	}
	return n
}

// probe connects anew every 20ms until stopped, and returns the span the web
// service did not answer.
func (s *restartServer) probe() func() string {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan string)
	go func() {
		hc := &http.Client{Timeout: time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
		start := time.Now()
		var first, last time.Time
		var errs []string
		for ctx.Err() == nil {
			resp, err := hc.Get(s.host + "/")
			if err == nil {
				_ = resp.Body.Close()
			} else {
				if first.IsZero() {
					first = time.Now()
				}
				last = time.Now()
				if len(errs) < 2 {
					errs = append(errs, err.Error())
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
		if first.IsZero() {
			done <- "never"
			return
		}
		done <- fmt.Sprintf("from %s to %s after the call (%v)", first.Sub(start).Round(time.Millisecond), last.Sub(start).Round(time.Millisecond), errs)
	}()
	return func() string {
		cancel()
		return <-done
	}
}
