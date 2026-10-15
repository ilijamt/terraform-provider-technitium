package technitium_test

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
	"github.com/ilijamt/terraform-provider-technitium/internal/provider"
	"github.com/ilijamt/terraform-provider-technitium/internal/technitium"
	"github.com/ilijamt/terraform-provider-technitium/internal/testserver"
)

var ownServer bool

func TestMain(m *testing.M) {
	if os.Getenv("TF_ACC") == "" {
		os.Exit(m.Run())
	}
	ownServer = os.Getenv("TECHNITIUM_TESTACC_HOST") == ""
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	host, stop, err := testserver.Start(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// os.Exit skips deferred calls, so an interrupted run would leak the container.
	go func() {
		<-ctx.Done()
		stop()
		os.Exit(1)
	}()
	_ = os.Setenv("TECHNITIUM_HOST", host)
	_ = os.Setenv("TECHNITIUM_USERNAME", testserver.Username)
	_ = os.Setenv("TECHNITIUM_PASSWORD", testserver.Password)
	_ = os.Unsetenv("TECHNITIUM_TOKEN")
	// OpenTofu resolves "ilijamt/technitium" to registry.opentofu.org and
	// rejects the harness's default registry.terraform.io/-/technitium.
	if os.Getenv("TF_ACC_PROVIDER_HOST") == "" && openTofu(os.Getenv("TF_ACC_TERRAFORM_PATH")) {
		_ = os.Setenv("TF_ACC_PROVIDER_HOST", "registry.opentofu.org")
	}
	code := m.Run()
	stop()
	os.Exit(code)
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

func TestSchemas(t *testing.T) {
	srv, err := factories["technitium"]()
	require.NoError(t, err)
	resp, err := srv.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	require.NoError(t, err)
	for _, d := range resp.Diagnostics {
		t.Errorf("%s: %s", d.Summary, d.Detail)
	}
	assert.Len(t, resp.ResourceSchemas, len(technitium.Resources()))
	assert.Len(t, resp.DataSourceSchemas, len(technitium.DataSources()))
	for _, name := range []string{"technitium_group", "technitium_permissions_zones", "technitium_settings_general", "technitium_zone"} {
		assert.Contains(t, resp.DataSourceSchemas, name)
	}
	assert.NotContains(t, resp.DataSourceSchemas, "technitium_record_a")
	for _, name := range []string{"technitium_settings_general", "technitium_zone", "technitium_record_a", "technitium_record_fwd"} {
		assert.Contains(t, resp.ResourceSchemas, name)
	}
}

func api(t *testing.T) *client.Client {
	c, err := login()
	require.NoError(t, err)
	return c
}

func login() (*client.Client, error) {
	return loginTo(os.Getenv("TECHNITIUM_HOST"))
}

func loginTo(host string) (*client.Client, error) {
	c := client.New(host, "", "test", false, nil)
	return c, c.Login(context.Background(), testserver.Username, testserver.Password)
}

// acceptance is for tests that set up the server before resource.Test,
// which skips on its own.
func acceptance(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run acceptance tests")
	}
}

func call(t *testing.T, c *client.Client, path string, params url.Values) map[string]any {
	t.Helper()
	resp, err := c.Call(context.Background(), path, params)
	require.NoError(t, err, path)
	return resp
}

func zonesDestroyed(zones ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		c, err := login()
		if err != nil {
			return err
		}
		for _, zone := range zones {
			_, err = c.Call(context.Background(), "/api/zones/options/get", url.Values{"zone": {zone}})
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

func TestAccZoneAndRecords(t *testing.T) {
	config := func(record string) string {
		return `
resource "technitium_zone" "test" {
  zone = "records.test"
}

resource "technitium_record_a" "www" {
  zone   = technitium_zone.test.zone
  domain = "www.records.test"
` + record + `
}

resource "technitium_record_cname" "app" {
  zone   = technitium_zone.test.zone
  domain = "app.records.test"
  cname  = technitium_record_a.www.domain
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("records.test"),
		Steps: []resource.TestStep{
			{
				Config: config(`ip_address = "10.0.0.1"
  ttl        = 300
  disable    = true`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone.test", "id", "records.test"),
					resource.TestCheckResourceAttr("technitium_zone.test", "type", "Primary"),
					resource.TestCheckResourceAttr("technitium_record_a.www", "id", "records.test/www.records.test/10.0.0.1"),
					resource.TestCheckResourceAttr("technitium_record_a.www", "ttl", "300"),
					resource.TestCheckResourceAttr("technitium_record_a.www", "disable", "true"),
					resource.TestCheckResourceAttr("technitium_record_cname.app", "cname", "www.records.test"),
				),
			},
			{
				Config: config(`ip_address = "10.0.0.1"
  ttl        = 300
  disable    = true
  comments   = "web"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_record_a.www", plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_a.www", "comments", "web"),
					resource.TestCheckResourceAttr("technitium_record_a.www", "ttl", "300"),
					resource.TestCheckResourceAttr("technitium_record_a.www", "disable", "true"),
				),
			},
			{
				Config: config(`ip_address = "10.0.0.2"
  ttl        = 300`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_record_a.www", plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_record_a.www", "ip_address", "10.0.0.2"),
			},
			{
				ResourceName:      "technitium_record_a.www",
				ImportState:       true,
				ImportStateId:     "records.test/www.records.test/10.0.0.2",
				ImportStateVerify: true,
			},
			{
				ResourceName:      "technitium_zone.test",
				ImportState:       true,
				ImportStateId:     "records.test",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccForwarderZone(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("178.168.192.in-addr.arpa"),
		Steps: []resource.TestStep{
			{
				Config: `
resource "technitium_zone" "reverse" {
  zone = "178.168.192.in-addr.arpa"
  type = "Forwarder"
}

resource "technitium_record_fwd" "reverse" {
  zone      = technitium_zone.reverse.zone
  domain    = technitium_zone.reverse.zone
  protocol  = "Udp"
  forwarder = "192.168.178.2"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone.reverse", "type", "Forwarder"),
					resource.TestCheckResourceAttr("technitium_record_fwd.reverse", "forwarder", "192.168.178.2"),
					func(*terraform.State) error {
						resp, err := api(t).Call(context.Background(), "/api/zones/records/get", url.Values{
							"zone": {"178.168.192.in-addr.arpa"}, "domain": {"178.168.192.in-addr.arpa"},
						})
						if err != nil {
							return err
						}
						var fwd []any
						for _, r := range resp["records"].([]any) {
							if r.(map[string]any)["type"] == "FWD" {
								fwd = append(fwd, r)
							}
						}
						if len(fwd) != 1 {
							return fmt.Errorf("want 1 FWD record, got %d: %v", len(fwd), fwd)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "technitium_record_fwd.reverse",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccZoneUpdateAndRemovedOutOfBand(t *testing.T) {
	config := func(disabled bool) string {
		return fmt.Sprintf(`
resource "technitium_zone" "test" {
  zone     = "drift.test"
  disabled = %t
}
`, disabled)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("drift.test"),
		Steps: []resource.TestStep{
			{
				Config: config(false),
				Check:  resource.TestCheckResourceAttr("technitium_zone.test", "disabled", "false"),
			},
			{
				Config: config(true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_zone.test", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_zone.test", "disabled", "true"),
			},
			{
				PreConfig: func() {
					_, err := api(t).Call(context.Background(), "/api/zones/delete", url.Values{"zone": {"drift.test"}})
					require.NoError(t, err)
				},
				Config: config(true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_zone.test", plancheck.ResourceActionCreate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_zone.test", "disabled", "true"),
			},
		},
	})
}

func TestAccTXTImportWithSlash(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("txt.test"),
		Steps: []resource.TestStep{
			{
				Config: `
resource "technitium_zone" "txt" {
  zone = "txt.test"
}

resource "technitium_record_txt" "spf" {
  zone   = technitium_zone.txt.zone
  domain = "txt.test"
  text   = "v=spf1 ip4:10.0.0.0/8 -all"
}
`,
				Check: resource.TestCheckResourceAttr("technitium_record_txt.spf", "text", "v=spf1 ip4:10.0.0.0/8 -all"),
			},
			{
				ResourceName:      "technitium_record_txt.spf",
				ImportState:       true,
				ImportStateId:     `{"zone": "txt.test", "domain": "txt.test", "text": "v=spf1 ip4:10.0.0.0/8 -all"}`,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccZoneListOptions(t *testing.T) {
	config := `
resource "technitium_zone" "acl" {
  zone                     = "acl.test"
  query_access             = "UseSpecifiedNetworkACL"
  query_access_network_acl = ["10.0.0.0/8", "!192.168.1.1"]
  notify                   = "SpecifiedNameServers"
  notify_name_servers      = ["192.0.2.1", "192.0.2.2"]
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("acl.test"),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone.acl", "query_access_network_acl.#", "2"),
					resource.TestCheckResourceAttr("technitium_zone.acl", "query_access_network_acl.1", "!192.168.1.1"),
					resource.TestCheckResourceAttr("technitium_zone.acl", "notify_name_servers.#", "2"),
				),
			},
			{
				ResourceName:      "technitium_zone.acl",
				ImportState:       true,
				ImportStateId:     "acl.test",
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					_, err := api(t).Call(context.Background(), "/api/zones/options/set", url.Values{"zone": {"acl.test"}, "queryAccessNetworkACL": {"10.1.0.0/16"}})
					require.NoError(t, err)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_zone.acl", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_zone.acl", "query_access_network_acl.0", "10.0.0.0/8"),
			},
		},
	})
}

func TestAccNAPTRAndRPRecords(t *testing.T) {
	config := func(ttl int) string {
		return fmt.Sprintf(`
resource "technitium_zone" "test" {
  zone = "naptr.test"
}

resource "technitium_record_naptr" "sip" {
  zone             = technitium_zone.test.zone
  domain           = "naptr.test"
  ttl              = %d
  naptr_order      = 100
  naptr_preference = 10
  naptr_flags      = "U"
  naptr_services   = "E2U+sip"
  naptr_regexp     = "!^.*$!sip:info@naptr.test!"
}

resource "technitium_record_rp" "admin" {
  zone       = technitium_zone.test.zone
  domain     = "naptr.test"
  mailbox    = "admin@naptr.test"
  txt_domain = "info.naptr.test"
}
`, ttl)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("naptr.test"),
		Steps: []resource.TestStep{
			{
				Config: config(300),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_naptr.sip", "naptr_regexp", "!^.*$!sip:info@naptr.test!"),
					resource.TestCheckResourceAttr("technitium_record_rp.admin", "mailbox", "admin@naptr.test"),
				),
			},
			{
				Config: config(600),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_record_naptr.sip", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_record_naptr.sip", "ttl", "600"),
			},
			{
				ResourceName:      "technitium_record_naptr.sip",
				ImportState:       true,
				ImportStateId:     `{"zone": "naptr.test", "domain": "naptr.test", "naptr_order": 100, "naptr_preference": 10, "naptr_flags": "U", "naptr_services": "E2U+sip", "naptr_regexp": "!^.*$!sip:info@naptr.test!"}`,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "technitium_record_rp.admin",
				ImportState:       true,
				ImportStateId:     "naptr.test/naptr.test/admin@naptr.test/info.naptr.test",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccSOARecord(t *testing.T) {
	config := func(minimum int, extra string) string {
		return fmt.Sprintf(`
resource "technitium_zone" "test" {
  zone = "soa.test"
}

resource "technitium_record_soa" "test" {
  zone    = technitium_zone.test.zone
  domain  = technitium_zone.test.zone
  refresh = 7200
  minimum = %d
}
%s`, minimum, extra)
	}
	// Adding a record bumps the serial, which must not show as drift.
	www := `
resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.soa.test"
  ip_address = "10.0.0.1"
  depends_on = [technitium_record_soa.test]
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("soa.test"),
		Steps: []resource.TestStep{
			{
				Config: config(300, ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_soa.test", "refresh", "7200"),
					resource.TestCheckResourceAttr("technitium_record_soa.test", "minimum", "300"),
					resource.TestCheckResourceAttrSet("technitium_record_soa.test", "serial"),
					resource.TestCheckResourceAttrSet("technitium_record_soa.test", "primary_name_server"),
				),
			},
			{
				Config: config(600, www),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_record_soa.test", plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_soa.test", "minimum", "600"),
					resource.TestCheckResourceAttr("technitium_record_soa.test", "refresh", "7200"),
				),
			},
			{
				ResourceName:      "technitium_record_soa.test",
				ImportState:       true,
				ImportStateId:     "soa.test/soa.test",
				ImportStateVerify: true,
				// The record added after the last refresh bumped it.
				ImportStateVerifyIgnore: []string{"serial"},
			},
			{
				Config: `
resource "technitium_zone" "test" {
  zone = "soa.test"
}
`,
				Check: func(*terraform.State) error {
					resp, err := api(t).Call(context.Background(), "/api/zones/records/get", url.Values{"zone": {"soa.test"}, "domain": {"soa.test"}})
					if err != nil {
						return err
					}
					for _, r := range resp["records"].([]any) {
						if rec := r.(map[string]any); rec["type"] == "SOA" {
							if got := fmt.Sprint(rec["rData"].(map[string]any)["minimum"]); got != "600" {
								return fmt.Errorf("minimum is %s", got)
							}
						}
					}
					return nil
				},
			},
		},
	})
}

// APP needs an installed DNS app, so it is tested on its own.
var recordTypes = map[string]string{
	"aaaa":  `domain = "v6.types.test"` + "\n" + `ip_address = "2001:db8::1"`,
	"ns":    `domain = "sub.types.test"` + "\n" + `name_server = "ns1.example.net"`,
	"mx":    `domain = "types.test"` + "\n" + `exchange = "mail.types.test"` + "\n" + `preference = 10`,
	"srv":   `domain = "_sip._tcp.types.test"` + "\n" + `priority = 10` + "\n" + `weight = 5` + "\n" + `port = 5060` + "\n" + `target = "sip.types.test"`,
	"dname": `domain = "old.types.test"` + "\n" + `dname = "new.example.net"`,
	"ds": `domain = "child.types.test"` + "\n" + `key_tag = 12345` + "\n" + `algorithm = "ECDSAP256SHA256"` + "\n" + `digest_type = "SHA256"` + "\n" +
		`digest = "2BB183AF5F22588179A53B0A98631FAD1A292118D3F04E8B8F6A1FB4A10A8D0D"`,
	"sshfp": `domain = "host.types.test"` + "\n" + `sshfp_algorithm = "Ed25519"` + "\n" + `sshfp_fingerprint_type = "SHA256"` + "\n" +
		`sshfp_fingerprint = "A3B1A6F2B1E9D7C3F0E5A8B2C4D6E8F0A1B3C5D7E9F1A3B5C7D9E1F3A5B7C9D1"`,
	"tlsa": `domain = "_443._tcp.types.test"` + "\n" + `tlsa_certificate_usage = "DANE-EE"` + "\n" + `tlsa_selector = "SPKI"` + "\n" +
		`tlsa_matching_type = "SHA2-256"` + "\n" + `tlsa_certificate_association_data = "0C72AC70B745AC19998811B131D662C9AC69DBDBE7CB23E5B514B56664C5D3D6"`,
	"svcb":  `domain = "_dns.types.test"` + "\n" + `svc_priority = 1` + "\n" + `svc_target_name = "dns.types.test"` + "\n" + `svc_params = { alpn = "h2" }`,
	"https": `domain = "www.types.test"` + "\n" + `svc_priority = 1` + "\n" + `svc_target_name = "www.types.test"` + "\n" + `svc_params = { alpn = "h2,h3", port = "8443" }`,
	"uri":   `domain = "_ftp._tcp.types.test"` + "\n" + `uri_priority = 10` + "\n" + `uri_weight = 1` + "\n" + `uri = "ftp://ftp.types.test/public"`,
	"caa":   `domain = "types.test"` + "\n" + `flags = 0` + "\n" + `tag = "issue"` + "\n" + `value = "letsencrypt.org"`,
	"aname": `domain = "alias.types.test"` + "\n" + `aname = "target.example.net"`,
	"ptr":   `zone = technitium_zone.reverse.zone` + "\n" + `domain = "1.2.0.192.in-addr.arpa"` + "\n" + `ptr_name = "host.types.test"`,
}

func TestAccRecordTypes(t *testing.T) {
	names := make([]string, 0, len(recordTypes))
	for name := range recordTypes {
		names = append(names, name)
	}
	sort.Strings(names)

	config := func(ttl int) string {
		var b strings.Builder
		b.WriteString(`
resource "technitium_zone" "types" {
  zone = "types.test"
}

resource "technitium_zone" "reverse" {
  zone = "2.0.192.in-addr.arpa"
}
`)
		for _, name := range names {
			body := recordTypes[name]
			if !strings.Contains(body, "zone = ") {
				body = "zone = technitium_zone.types.zone\n" + body
			}
			fmt.Fprintf(&b, "\nresource \"technitium_record_%s\" \"test\" {\n%s\nttl = %d\n}\n", name, body, ttl)
		}
		return b.String()
	}

	var updates []plancheck.PlanCheck
	var imports []resource.TestStep
	for _, name := range names {
		addr := "technitium_record_" + name + ".test"
		updates = append(updates, plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate))
		imports = append(imports, resource.TestStep{ResourceName: addr, ImportState: true, ImportStateVerify: true})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("types.test", "2.0.192.in-addr.arpa"),
		Steps: append([]resource.TestStep{
			{Config: config(300)},
			{
				Config:           config(600),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: updates},
				Check:            resource.TestCheckResourceAttr("technitium_record_mx.test", "ttl", "600"),
			},
		}, imports...),
	})
}

func TestAccZoneUpdateSecurityPolicies(t *testing.T) {
	config := func(policies string) string {
		return `
resource "technitium_zone" "test" {
  zone                     = "policy.test"
  update                   = "Allow"
  update_security_policies = [` + policies + `]
}
`
	}
	policies := `
    { tsig_key_name = "update-key", domain = "policy.test", allowed_types = ["A", "AAAA"] },
    { tsig_key_name = "update-key", domain = "*.policy.test", allowed_types = ["TXT"] },
  `
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("policy.test"),
		Steps: []resource.TestStep{
			{
				Config: config(policies),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone.test", "update_security_policies.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("technitium_zone.test", "update_security_policies.*", map[string]string{
						"domain": "*.policy.test", "allowed_types.#": "1", "allowed_types.0": "TXT",
					}),
				),
			},
			{
				PreConfig: func() {
					_, err := api(t).Call(context.Background(), "/api/zones/options/set", url.Values{
						"zone": {"policy.test"}, "updateSecurityPolicies": {"update-key|policy.test|A"},
					})
					require.NoError(t, err)
				},
				Config: config(policies),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_zone.test", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_zone.test", "update_security_policies.#", "2"),
			},
			{
				ResourceName:      "technitium_zone.test",
				ImportState:       true,
				ImportStateId:     "policy.test",
				ImportStateVerify: true,
			},
			{
				Config: config(""),
				Check:  resource.TestCheckResourceAttr("technitium_zone.test", "update_security_policies.#", "0"),
			},
		},
	})
}

func TestAccSecondaryZones(t *testing.T) {
	acceptance(t)
	if !ownServer {
		t.Skip("needs the test server in Docker to reach a second server; TECHNITIUM_TESTACC_HOST is set")
	}
	host, ip, stop, err := testserver.Run(context.Background())
	require.NoError(t, err)
	t.Cleanup(stop)
	primary, err := loginTo(host)
	require.NoError(t, err)
	for zone, typ := range map[string]string{"secondary.test": "Primary", "stub.test": "Primary", "secfwd.test": "Forwarder"} {
		call(t, primary, "/api/zones/create", url.Values{"zone": {zone}, "type": {typ}, "initializeForwarder": {"false"}})
		call(t, primary, "/api/zones/options/set", url.Values{"zone": {zone}, "zoneTransfer": {"Allow"}})
	}
	call(t, primary, "/api/zones/records/add", url.Values{"zone": {"secondary.test"}, "domain": {"www.secondary.test"}, "type": {"A"}, "ipAddress": {"10.0.0.1"}})

	// The server adds the port for Tls and Quic, so the config carries it.
	config := func(protocol, port string) string {
		return fmt.Sprintf(`
resource "technitium_zone" "secondary" {
  zone                           = "secondary.test"
  type                           = "Secondary"
  primary_name_server_addresses  = ["%[1]s%[3]s"]
  primary_zone_transfer_protocol = "%[2]s"
}

resource "technitium_zone" "stub" {
  zone                          = "stub.test"
  type                          = "Stub"
  primary_name_server_addresses = ["%[1]s"]
}

resource "technitium_zone" "secfwd" {
  zone                          = "secfwd.test"
  type                          = "SecondaryForwarder"
  primary_name_server_addresses = ["%[1]s"]
}
`, ip, protocol, port)
	}
	// The first transfer runs in the background after create.
	transferred := func(*terraform.State) error {
		for range 60 {
			resp, err := api(t).Call(context.Background(), "/api/zones/records/get", url.Values{"zone": {"secondary.test"}, "domain": {"www.secondary.test"}})
			if err == nil && len(resp["records"].([]any)) > 0 {
				return nil
			}
			time.Sleep(500 * time.Millisecond)
		}
		return fmt.Errorf("www.secondary.test was not transferred from %s", ip)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("secondary.test", "stub.test", "secfwd.test"),
		Steps: []resource.TestStep{
			{
				Config: config("Tcp", ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone.secondary", "type", "Secondary"),
					resource.TestCheckResourceAttr("technitium_zone.secondary", "primary_zone_transfer_protocol", "Tcp"),
					resource.TestCheckResourceAttr("technitium_zone.stub", "type", "Stub"),
					resource.TestCheckResourceAttr("technitium_zone.secfwd", "type", "SecondaryForwarder"),
					transferred,
				),
			},
			{
				Config: config("Tls", ":853"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_zone.secondary", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_zone.secondary", "primary_zone_transfer_protocol", "Tls"),
			},
			{
				ResourceName:      "technitium_zone.secondary",
				ImportState:       true,
				ImportStateId:     "secondary.test",
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccAppRecord installs the Split Horizon app from the DNS App Store, so
// it needs the test server to reach the internet.
func TestAccAppRecord(t *testing.T) {
	acceptance(t)
	c := api(t)
	resp, err := c.Call(context.Background(), "/api/apps/listStoreApps", nil)
	if err != nil {
		t.Skipf("DNS App Store unreachable: %s", err)
	}
	var appURL string
	for _, a := range resp["storeApps"].([]any) {
		if app := a.(map[string]any); app["name"] == "Split Horizon" {
			appURL, _ = app["url"].(string)
		}
	}
	require.NotEmpty(t, appURL, "Split Horizon is not in the DNS App Store")
	call(t, c, "/api/apps/downloadAndInstall", url.Values{"name": {"Split Horizon"}, "url": {appURL}})
	t.Cleanup(func() {
		_, _ = c.Call(context.Background(), "/api/apps/uninstall", url.Values{"name": {"Split Horizon"}})
	})

	config := func(private string) string {
		return fmt.Sprintf(`
resource "technitium_zone" "test" {
  zone = "app.test"
}

resource "technitium_record_app" "www" {
  zone        = technitium_zone.test.zone
  domain      = "www.app.test"
  app_name    = "Split Horizon"
  class_path  = "SplitHorizon.SimpleAddress"
  record_data = jsonencode({ public = ["192.0.2.1"], private = ["%s"] })
}
`, private)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("app.test"),
		Steps: []resource.TestStep{
			{
				Config: config("10.0.0.1"),
				Check:  resource.TestCheckResourceAttr("technitium_record_app.www", "record_data", `{"private":["10.0.0.1"],"public":["192.0.2.1"]}`),
			},
			{
				Config: config("10.0.0.2"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_record_app.www", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_record_app.www", "record_data", `{"private":["10.0.0.2"],"public":["192.0.2.1"]}`),
			},
			{
				PreConfig: func() {
					call(t, c, "/api/zones/records/update", url.Values{
						"zone": {"app.test"}, "domain": {"www.app.test"}, "type": {"APP"},
						"appName": {"Split Horizon"}, "classPath": {"SplitHorizon.SimpleAddress"}, "recordData": {`{"public":[],"private":[]}`},
					})
				},
				Config: config("10.0.0.2"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_record_app.www", plancheck.ResourceActionUpdate)},
				},
			},
			{
				ResourceName:      "technitium_record_app.www",
				ImportState:       true,
				ImportStateId:     "app.test/www.app.test",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccDHCP(t *testing.T) {
	scope := func(end, extra string) string {
		return fmt.Sprintf(`
resource "technitium_dhcp_scope" "lan" {
  name             = "lan"
  starting_address = "192.168.50.10"
  ending_address   = "%s"
  subnet_mask      = "255.255.255.0"
  router_address   = "192.168.50.1"
  dns_servers      = ["192.168.50.2", "192.168.50.3"]
  domain_name      = "lan.test"
  %s
}
`, end, extra)
	}
	tables := `
  exclusions      = [{ starting_address = "192.168.50.10", ending_address = "192.168.50.20" }]
  static_routes   = [{ destination = "172.16.0.0", subnet_mask = "255.255.0.0", router = "192.168.50.254" }]
  generic_options = [{ code = 150, value = "C0:A8:32:05" }]
  vendor_info     = [{ identifier = "PXEClient", information = "06:01:03" }]
`
	lease := func(mac string) string {
		return fmt.Sprintf(`
resource "technitium_dhcp_reserved_lease" "printer" {
  scope            = technitium_dhcp_scope.lan.name
  hardware_address = "%s"
  ip_address       = "192.168.50.100"
  host_name        = "printer"
  comments         = "office"
}
`, mac)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(*terraform.State) error {
			c, err := login()
			if err != nil {
				return err
			}
			if _, err = c.Call(context.Background(), "/api/dhcp/scopes/get", url.Values{"name": {"lan"}}); err == nil {
				return fmt.Errorf("scope lan still exists")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config:      scope("192.168.50.200", "") + lease("00:11:22:aa:bb:cc"),
				ExpectError: regexp.MustCompile(`must match`),
			},
			{
				Config: scope("192.168.50.200", tables) + lease("00-11-22-AA-BB-CC"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_dhcp_scope.lan", "dns_servers.#", "2"),
					resource.TestCheckResourceAttr("technitium_dhcp_scope.lan", "exclusions.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs("technitium_dhcp_scope.lan", "generic_options.*", map[string]string{"code": "150", "value": "C0:A8:32:05"}),
					resource.TestCheckResourceAttrSet("technitium_dhcp_scope.lan", "lease_time_days"),
					resource.TestCheckResourceAttr("technitium_dhcp_reserved_lease.printer", "ip_address", "192.168.50.100"),
				),
			},
			{
				Config: scope("192.168.50.150", tables) + lease("00-11-22-AA-BB-CC"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("technitium_dhcp_scope.lan", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("technitium_dhcp_reserved_lease.printer", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.TestCheckResourceAttr("technitium_dhcp_scope.lan", "ending_address", "192.168.50.150"),
			},
			{
				PreConfig: func() {
					call(t, api(t), "/api/dhcp/scopes/set", url.Values{"name": {"lan"}, "dnsServers": {"192.168.50.9"}})
				},
				Config: scope("192.168.50.150", tables) + lease("00-11-22-AA-BB-CC"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_dhcp_scope.lan", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_dhcp_scope.lan", "dns_servers.#", "2"),
			},
			{
				ResourceName:      "technitium_dhcp_scope.lan",
				ImportState:       true,
				ImportStateId:     "lan",
				ImportStateVerify: true,
			},
			{
				ResourceName:      "technitium_dhcp_reserved_lease.printer",
				ImportState:       true,
				ImportStateId:     "lan/00-11-22-AA-BB-CC",
				ImportStateVerify: true,
			},
			{
				Config: scope("192.168.50.150", `
  exclusions      = []
  static_routes   = []
  generic_options = []
  vendor_info     = []
`) + lease("00-11-22-AA-BB-CC"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_dhcp_scope.lan", "exclusions.#", "0"),
					resource.TestCheckResourceAttr("technitium_dhcp_scope.lan", "generic_options.#", "0"),
				),
			},
		},
	})
}

func TestAccAllowedAndBlockedZones(t *testing.T) {
	config := func(blocked string) string {
		return fmt.Sprintf(`
resource "technitium_blocked_zone" "ads" {
  domain = "%s"
}

resource "technitium_allowed_zone" "cdn" {
  domain = "cdn.ads.block.test"
}
`, blocked)
	}
	listed := func(kind, domain string) bool {
		resp := call(t, api(t), "/api/"+kind+"/list", url.Values{"domain": {domain}})
		for _, r := range resp["records"].([]any) {
			if r.(map[string]any)["name"] == domain {
				return true
			}
		}
		return false
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(*terraform.State) error {
			for kind, domain := range map[string]string{"blocked": "tracker.block.test", "allowed": "cdn.ads.block.test"} {
				if listed(kind, domain) {
					return fmt.Errorf("%s is still %s", domain, kind)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config:      config("ads.block.test."),
				ExpectError: regexp.MustCompile(`must match`),
			},
			{
				Config: config("ads.block.test"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_blocked_zone.ads", "id", "ads.block.test"),
					func(*terraform.State) error {
						if !listed("blocked", "ads.block.test") || !listed("allowed", "cdn.ads.block.test") {
							return fmt.Errorf("entries missing on the server")
						}
						return nil
					},
				),
			},
			{
				PreConfig: func() { call(t, api(t), "/api/blocked/delete", url.Values{"domain": {"ads.block.test"}}) },
				Config:    config("ads.block.test"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_blocked_zone.ads", plancheck.ResourceActionCreate)},
				},
			},
			{
				Config: config("tracker.block.test"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_blocked_zone.ads", plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: func(*terraform.State) error {
					if listed("blocked", "ads.block.test") {
						return fmt.Errorf("ads.block.test is still blocked")
					}
					return nil
				},
			},
			{
				ResourceName:      "technitium_blocked_zone.ads",
				ImportState:       true,
				ImportStateId:     "tracker.block.test",
				ImportStateVerify: true,
			},
			{
				ResourceName:      "technitium_allowed_zone.cdn",
				ImportState:       true,
				ImportStateId:     "cdn.ads.block.test",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccUsersGroupsAndPermissions(t *testing.T) {
	config := func(username, password string) string {
		return fmt.Sprintf(`
resource "technitium_user" "alice" {
  username     = "%s"
  password     = "%s"
  display_name = "Alice"
}

resource "technitium_group" "ops" {
  name        = "Ops"
  description = "Operations"
}

resource "technitium_group_members" "ops" {
  group   = technitium_group.ops.name
  members = [technitium_user.alice.username]
}

resource "technitium_permissions_zones" "this" {
  user_permissions = [
    { username = technitium_user.alice.username, can_view = true, can_modify = false, can_delete = false },
  ]
  group_permissions = [
    { name = technitium_group.ops.name, can_view = true, can_modify = true, can_delete = false },
  ]
}
`, username, password)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(*terraform.State) error {
			c, err := login()
			if err != nil {
				return err
			}
			if _, err = c.Call(context.Background(), "/api/admin/users/get", url.Values{"user": {"alice"}}); err == nil {
				return fmt.Errorf("user alice still exists")
			}
			if _, err = c.Call(context.Background(), "/api/admin/groups/get", url.Values{"group": {"Ops"}}); err == nil {
				return fmt.Errorf("group Ops still exists")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config:      config("Alice", "first-secret"),
				ExpectError: regexp.MustCompile(`must match`),
			},
			{
				Config: config("alice", "first-secret"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_group_members.ops", "members.0", "alice"),
					resource.TestCheckResourceAttr("technitium_permissions_zones.this", "group_permissions.#", "1"),
					resource.TestCheckResourceAttr("technitium_user.alice", "session_timeout_seconds", "1800"),
				),
			},
			{
				Config: config("alice", "second-secret"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_user.alice", plancheck.ResourceActionUpdate)},
				},
				Check: func(*terraform.State) error {
					c := client.New(os.Getenv("TECHNITIUM_HOST"), "", "test", false, nil)
					return c.Login(context.Background(), "alice", "second-secret")
				},
			},
			{
				PreConfig: func() { call(t, api(t), "/api/admin/groups/set", url.Values{"group": {"Ops"}, "members": {""}}) },
				Config:    config("alice", "second-secret"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_group_members.ops", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_group_members.ops", "members.#", "1"),
			},
			{
				ResourceName:            "technitium_user.alice",
				ImportState:             true,
				ImportStateId:           "alice",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			{
				ResourceName:      "technitium_group.ops",
				ImportState:       true,
				ImportStateId:     "Ops",
				ImportStateVerify: true,
			},
			{
				ResourceName:      "technitium_group_members.ops",
				ImportState:       true,
				ImportStateId:     "Ops",
				ImportStateVerify: true,
			},
			{
				ResourceName:      "technitium_permissions_zones.this",
				ImportState:       true,
				ImportStateId:     "permissions_zones",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccDataSources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("data.test"),
		Steps: []resource.TestStep{
			{
				Config: `
data "technitium_group" "nope" {
  name = "No Such Group"
}
`,
				ExpectError: regexp.MustCompile(`not found`),
			},
			{
				Config: `
resource "technitium_zone" "test" {
  zone = "data.test"
}

data "technitium_zone" "test" {
  zone = technitium_zone.test.zone
}

data "technitium_group" "admins" {
  name = "Administrators"
}

data "technitium_user" "admin" {
  username = "admin"
}

data "technitium_settings_general" "this" {}

resource "technitium_dhcp_scope" "test" {
  name             = "data"
  starting_address = "192.168.80.10"
  ending_address   = "192.168.80.20"
  subnet_mask      = "255.255.255.0"
}

data "technitium_dhcp_scope" "test" {
  name = technitium_dhcp_scope.test.name
}

resource "technitium_group" "ops" {
  name = "DataOps"
}

# The built-in groups the server keeps need not be listed.
resource "technitium_permissions_zones" "this" {
  group_permissions = [{ name = technitium_group.ops.name, can_view = true, can_modify = false, can_delete = false }]
}

data "technitium_permissions_zones" "this" {
  depends_on = [technitium_permissions_zones.this]
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.technitium_zone.test", "type", "Primary"),
					resource.TestCheckResourceAttr("data.technitium_group.admins", "description", "Super administrators"),
					resource.TestCheckResourceAttr("data.technitium_user.admin", "display_name", "Administrator"),
					resource.TestCheckNoResourceAttr("data.technitium_user.admin", "password"),
					resource.TestCheckResourceAttr("data.technitium_user.admin", "is_sso_user", "false"),
					resource.TestCheckResourceAttrSet("data.technitium_settings_general.this", "dns_server_domain"),
					resource.TestCheckResourceAttr("data.technitium_dhcp_scope.test", "starting_address", "192.168.80.10"),
					resource.TestCheckTypeSetElemNestedAttrs("data.technitium_permissions_zones.this", "group_permissions.*", map[string]string{"name": "DataOps"}),
					resource.TestCheckResourceAttr("data.technitium_permissions_zones.this", "group_permissions.#", "1"),
					resource.TestCheckResourceAttr("technitium_permissions_zones.this", "group_permissions.#", "1"),
				),
			},
		},
	})
}

func TestAccServiceBindingSelf(t *testing.T) {
	config := func(ttl int) string {
		return fmt.Sprintf(`
resource "technitium_zone" "test" {
  zone = "self.test"
}

resource "technitium_record_https" "self" {
  zone            = technitium_zone.test.zone
  domain          = "self.test"
  svc_priority    = 1
  svc_target_name = "."
  svc_params      = {}
  ttl             = %d
}
`, ttl)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("self.test"),
		Steps: []resource.TestStep{
			{
				Config: config(300),
				Check:  resource.TestCheckResourceAttr("technitium_record_https.self", "svc_target_name", "."),
			},
			{
				Config: config(600),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_record_https.self", plancheck.ResourceActionUpdate)},
				},
			},
			{
				ResourceName:      "technitium_record_https.self",
				ImportState:       true,
				ImportStateId:     `{"zone": "self.test", "domain": "self.test", "svc_priority": 1, "svc_target_name": ".", "svc_params": {}}`,
				ImportStateVerify: true,
			},
		},
	})
}

func resetSSO(t *testing.T) {
	t.Cleanup(func() {
		c, err := login()
		if err == nil {
			_, _ = c.Call(context.Background(), "/api/admin/sso/set", url.Values{
				"ssoEnabled": {"false"}, "ssoAuthority": {""}, "ssoClientId": {""}, "ssoClientSecret": {""},
				"ssoAllowSignup": {"false"}, "ssoGroupMap": {""},
			})
			// The change restarts the web service a moment later; let it
			// finish before the next test calls the server.
			time.Sleep(2 * time.Second)
			_, _ = login()
		}
	})
}

// SSO stays disabled: enabling it would make the server restart its web
// service against a provider that does not exist.
func TestAccSettingsSSO(t *testing.T) {
	acceptance(t)
	resetSSO(t)
	config := func(remote string) string {
		return `
data "technitium_group" "dns_admins" {
  name = "DNS Administrators"
}

resource "technitium_settings_sso" "this" {
  enabled       = false
  authority     = "https://idp.example.com/realms/home"
  client_id     = "technitium"
  client_secret = "s3cret"
  allow_signup  = true
  group_map = [
    { remote_group = "` + remote + `", local_group = data.technitium_group.dns_admins.name },
  ]
}

data "technitium_settings_sso" "this" {
  depends_on = [technitium_settings_sso.this]
}
`
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config("dns-admins"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_settings_sso.this", "client_id", "technitium"),
					resource.TestCheckResourceAttr("technitium_settings_sso.this", "scopes.#", "3"),
					resource.TestCheckResourceAttr("data.technitium_settings_sso.this", "authority", "https://idp.example.com/realms/home"),
					resource.TestCheckNoResourceAttr("data.technitium_settings_sso.this", "client_secret"),
					resource.TestCheckTypeSetElemNestedAttrs("data.technitium_settings_sso.this", "group_map.*", map[string]string{"remote_group": "dns-admins", "local_group": "DNS Administrators"}),
				),
			},
			{
				Config: config("dns-operators"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_settings_sso.this", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckTypeSetElemNestedAttrs("technitium_settings_sso.this", "group_map.*", map[string]string{"remote_group": "dns-operators"}),
			},
			{
				ResourceName:            "technitium_settings_sso.this",
				ImportState:             true,
				ImportStateId:           "settings_sso",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"client_secret"},
			},
		},
	})
}

func TestAccKeptGroupNeedsFullRights(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: `
resource "technitium_permissions_dashboard" "this" {
  group_permissions = [{ name = "Administrators", can_view = true, can_modify = false, can_delete = false }]
}
`,
			ExpectError: regexp.MustCompile(`always has full rights`),
		}},
	})
}

func TestAccZonePermissions(t *testing.T) {
	config := `
resource "technitium_zone" "test" {
  zone = "perm.test"
}

resource "technitium_user" "bob" {
  username = "bob"
  password = "bob-secret"
}

resource "technitium_group" "ops" {
  name = "ZoneOps"
}

resource "technitium_zone_permissions" "test" {
  zone = technitium_zone.test.zone
  user_permissions = [
    { username = technitium_user.bob.username, can_view = true, can_modify = true, can_delete = false },
  ]
  group_permissions = [
    { name = technitium_group.ops.name, can_view = true, can_modify = false, can_delete = false },
  ]
}

data "technitium_zone_permissions" "test" {
  zone       = technitium_zone_permissions.test.zone
  depends_on = [technitium_zone_permissions.test]
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("perm.test"),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone_permissions.test", "group_permissions.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs("data.technitium_zone_permissions.test", "user_permissions.*", map[string]string{"username": "bob", "can_modify": "true"}),
				),
			},
			{
				PreConfig: func() {
					call(t, api(t), "/api/zones/permissions/set", url.Values{"zone": {"perm.test"}, "userPermissions": {""}})
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_zone_permissions.test", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_zone_permissions.test", "user_permissions.#", "1"),
			},
			{
				ResourceName:      "technitium_zone_permissions.test",
				ImportState:       true,
				ImportStateId:     "perm.test",
				ImportStateVerify: true,
			},
			{
				Config: strings.Replace(config, `group_permissions = [`, `group_permissions = [
    { name = "Administrators", can_view = true, can_modify = true, can_delete = true },
    { name = "DNS Administrators", can_view = true, can_modify = true, can_delete = true },`, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_zone_permissions.test", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_zone_permissions.test", "group_permissions.#", "3"),
			},
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("technitium_zone_permissions.test", "group_permissions.#", "1"),
			},
		},
	})
}

func storeURL(t *testing.T, name string) string {
	resp, err := api(t).Call(context.Background(), "/api/apps/listStoreApps", nil)
	if err != nil {
		t.Skipf("DNS App Store unreachable: %s", err)
	}
	return appURL(t, resp, name)
}

func tryStoreURL(t *testing.T, name string) string {
	resp, err := api(t).Call(context.Background(), "/api/apps/listStoreApps", nil)
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

func TestAccApps(t *testing.T) {
	acceptance(t)
	storeURL(t, "Split Horizon") // skips the test without the DNS App Store
	config := func(lan string) string {
		return fmt.Sprintf(`
data "technitium_store_app" "split_horizon" {
  name = "Split Horizon"
}

resource "technitium_app" "split_horizon" {
  name = data.technitium_store_app.split_horizon.name
  url  = data.technitium_store_app.split_horizon.url
}

resource "technitium_app_config" "split_horizon" {
  name   = technitium_app.split_horizon.name
  config = jsonencode({ networks = { lan = ["%s"] } })
}

resource "technitium_zone" "test" {
  zone = "apps.test"
}

resource "technitium_record_app" "www" {
  zone        = technitium_zone.test.zone
  domain      = "www.apps.test"
  app_name    = technitium_app.split_horizon.name
  class_path  = "SplitHorizon.SimpleAddress"
  record_data = jsonencode({ lan = ["10.0.0.1"] })
}
`, lan)
	}
	installed := func() (bool, error) {
		c, err := login()
		if err != nil {
			return false, err
		}
		resp, err := c.Call(context.Background(), "/api/apps/list", nil)
		if err != nil {
			return false, err
		}
		for _, a := range resp["apps"].([]any) {
			if a.(map[string]any)["name"] == "Split Horizon" {
				return true, nil
			}
		}
		return false, nil
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(*terraform.State) error {
			if ok, err := installed(); err != nil || ok {
				return fmt.Errorf("app still installed (%v)", err)
			}
			return zonesDestroyed("apps.test")(nil)
		},
		Steps: []resource.TestStep{
			{
				Config: config("192.168.0.0/16"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("technitium_app.split_horizon", "version"),
					resource.TestCheckResourceAttrPair("technitium_app.split_horizon", "version", "data.technitium_store_app.split_horizon", "version"),
					resource.TestCheckResourceAttr("technitium_app_config.split_horizon", "config", `{"networks":{"lan":["192.168.0.0/16"]}}`),
				),
			},
			{
				Config: config("10.0.0.0/8"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("technitium_app_config.split_horizon", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("technitium_app.split_horizon", plancheck.ResourceActionNoop),
					},
				},
			},
			{
				PreConfig: func() {
					call(t, api(t), "/api/apps/config/set", url.Values{"name": {"Split Horizon"}, "config": {"{\r\n  \"networks\": {\r\n    \"lan\": [ \"10.0.0.0/8\" ]\r\n  }\r\n}\r\n"}})
				},
				Config: config("10.0.0.0/8"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_app_config.split_horizon", plancheck.ResourceActionNoop)},
				},
			},
			{
				ResourceName:            "technitium_app.split_horizon",
				ImportState:             true,
				ImportStateId:           "Split Horizon",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"url"},
			},
			{
				ResourceName:      "technitium_app_config.split_horizon",
				ImportState:       true,
				ImportStateId:     "Split Horizon",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccDHCPScopeEnabled(t *testing.T) {
	config := func(enabled bool) string {
		return fmt.Sprintf(`
resource "technitium_dhcp_scope" "toggle" {
  name             = "toggle"
  starting_address = "192.168.90.10"
  ending_address   = "192.168.90.20"
  subnet_mask      = "255.255.255.0"
  enabled          = %t
}

data "technitium_dhcp_scope" "toggle" {
  name       = technitium_dhcp_scope.toggle.name
  depends_on = [technitium_dhcp_scope.toggle]
}
`, enabled)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				// A new scope starts enabled, so create disables it.
				Config: config(false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_dhcp_scope.toggle", "enabled", "false"),
					resource.TestCheckResourceAttr("data.technitium_dhcp_scope.toggle", "enabled", "false"),
				),
			},
			{
				Config: config(true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_dhcp_scope.toggle", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_dhcp_scope.toggle", "enabled", "true"),
			},
			{
				PreConfig: func() { call(t, api(t), "/api/dhcp/scopes/disable", url.Values{"name": {"toggle"}}) },
				Config:    config(true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_dhcp_scope.toggle", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_dhcp_scope.toggle", "enabled", "true"),
			},
			{
				ResourceName:      "technitium_dhcp_scope.toggle",
				ImportState:       true,
				ImportStateId:     "toggle",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccAPIToken(t *testing.T) {
	var token string
	works := func(token string) error {
		_, err := client.New(os.Getenv("TECHNITIUM_HOST"), token, "test", false, nil).Call(context.Background(), "/api/user/session/get", nil)
		return err
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(*terraform.State) error {
			if works(token) == nil {
				return fmt.Errorf("the token still works after destroy")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: `
resource "technitium_user" "ci" {
  username = "ci"
  password = "ci-secret"
}

resource "technitium_api_token" "ci" {
  username   = technitium_user.ci.username
  token_name = "deploy"
}
`,
				Check: resource.ComposeTestCheckFunc(
					func(s *terraform.State) error {
						attrs := s.RootModule().Resources["technitium_api_token.ci"].Primary.Attributes
						token = attrs["token"]
						if len(token) < 16 || attrs["partial_token"] != token[:16] || attrs["id"] != token[:16] {
							return fmt.Errorf("partial token %q does not start the token", attrs["partial_token"])
						}
						return works(token)
					},
				),
			},
			{
				ResourceName:            "technitium_api_token.ci",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"token"},
			},
		},
	})
}

func TestAccZoneDNSSEC(t *testing.T) {
	config := func(nx string, rollover int) string {
		return fmt.Sprintf(`
resource "technitium_zone" "test" {
  zone = "signed.test"
}

resource "technitium_zone_dnssec" "test" {
  zone              = technitium_zone.test.zone
  algorithm         = "ECDSA"
  curve             = "P256"
  nx_proof          = "%s"
  zsk_rollover_days = %d
}

data "technitium_zone_dnssec" "test" {
  zone       = technitium_zone_dnssec.test.zone
  depends_on = [technitium_zone_dnssec.test]
}
`, nx, rollover)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("signed.test"),
		Steps: []resource.TestStep{
			{
				Config: config("NSEC3", 30),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "status", "SignedWithNSEC3"),
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "nsec3_iterations", "0"),
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "keys.#", "2"),
					resource.TestCheckResourceAttrSet("technitium_zone_dnssec.test", "ds_records.0.digest"),
					resource.TestCheckResourceAttr("data.technitium_zone_dnssec.test", "signed", "true"),
					resource.TestCheckResourceAttr("data.technitium_zone_dnssec.test", "curve", "P256"),
				),
			},
			{
				Config: config("NSEC", 60),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_zone_dnssec.test", plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "status", "SignedWithNSEC"),
					resource.TestCheckResourceAttr("technitium_zone_dnssec.test", "zsk_rollover_days", "60"),
					resource.TestCheckNoResourceAttr("technitium_zone_dnssec.test", "nsec3_iterations"),
				),
			},
			{
				ResourceName:      "technitium_zone_dnssec.test",
				ImportState:       true,
				ImportStateId:     "signed.test",
				ImportStateVerify: true,
			},
			{
				Config: `
resource "technitium_zone" "test" {
  zone = "signed.test"
}
`,
				Check: func(*terraform.State) error {
					resp := call(t, api(t), "/api/zones/dnssec/properties/get", url.Values{"zone": {"signed.test"}})
					if resp["dnssecStatus"] != "Unsigned" {
						return fmt.Errorf("zone is %v after destroy", resp["dnssecStatus"])
					}
					return nil
				},
			},
		},
	})
}

func resetLDAP(t *testing.T) {
	t.Cleanup(func() {
		if c, err := login(); err == nil {
			_, _ = c.Call(context.Background(), "/api/admin/ldap/set", url.Values{
				"ldapEnabled": {"false"}, "ldapServer": {""}, "ldapBindUsername": {""}, "ldapBindPassword": {""},
				"ldapSearchBase": {""}, "ldapUserSearchFilter": {""}, "ldapGroupAttribute": {""}, "ldapGroupMap": {""}, "ldapAllowSignup": {"false"},
			})
		}
	})
}

// LDAP stays disabled: there is no directory to sign in against.
func TestAccSettingsLDAP(t *testing.T) {
	acceptance(t)
	resetLDAP(t)
	config := func(group string) string {
		return `
data "technitium_group" "dns_admins" {
  name = "DNS Administrators"
}

resource "technitium_settings_ldap" "this" {
  enabled            = false
  server             = "ldap.example.com"
  port               = 636
  ssl_option         = "LDAPS"
  bind_username      = "CN=svc,DC=example,DC=com"
  bind_password      = "s3cret"
  search_base        = "DC=example,DC=com"
  user_search_filter = "(sAMAccountName={0})"
  group_attribute    = "memberOf"
  group_map = [
    { remote_group = "` + group + `", local_group = data.technitium_group.dns_admins.name },
  ]
}

data "technitium_settings_ldap" "this" {
  depends_on = [technitium_settings_ldap.this]
}
`
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config("CN=dns,DC=example,DC=com"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_settings_ldap.this", "port", "636"),
					resource.TestCheckResourceAttr("data.technitium_settings_ldap.this", "ssl_option", "LDAPS"),
					resource.TestCheckNoResourceAttr("data.technitium_settings_ldap.this", "bind_password"),
					resource.TestCheckTypeSetElemNestedAttrs("data.technitium_settings_ldap.this", "group_map.*", map[string]string{"local_group": "DNS Administrators"}),
				),
			},
			{
				Config: config("CN=dns-ops,DC=example,DC=com"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_settings_ldap.this", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckTypeSetElemNestedAttrs("technitium_settings_ldap.this", "group_map.*", map[string]string{"remote_group": "CN=dns-ops,DC=example,DC=com"}),
			},
			{
				ResourceName:            "technitium_settings_ldap.this",
				ImportState:             true,
				ImportStateId:           "settings_ldap",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"bind_password"},
			},
		},
	})
}

func TestAccRecordRemovedOutOfBand(t *testing.T) {
	config := `
resource "technitium_zone" "test" {
  zone = "gone.test"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.gone.test"
  ip_address = "10.0.0.1"
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("gone.test"),
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig: func() {
					_, err := api(t).Call(context.Background(), "/api/zones/records/delete", url.Values{
						"zone": {"gone.test"}, "domain": {"www.gone.test"}, "type": {"A"}, "ipAddress": {"10.0.0.1"},
					})
					require.NoError(t, err)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_record_a.www", plancheck.ResourceActionCreate)},
				},
			},
		},
	})
}

func TestAccSettings(t *testing.T) {
	config := func(forwarders string) string {
		return `
resource "technitium_settings_general" "this" {
  dns_server_domain = "ns.settings.test"
}

resource "technitium_settings_proxy_forwarders" "this" {
  forwarders = ` + forwarders + `
}

resource "technitium_settings_recursion" "this" {
  recursion = "Allow"
}
`
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config(`["1.1.1.1", "9.9.9.9"]`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_settings_general.this", "id", "settings_general"),
					resource.TestCheckResourceAttr("technitium_settings_general.this", "dns_server_domain", "ns.settings.test"),
					resource.TestCheckResourceAttr("technitium_settings_proxy_forwarders.this", "forwarders.#", "2"),
					resource.TestCheckResourceAttr("technitium_settings_recursion.this", "recursion", "Allow"),
				),
			},
			{
				Config: config(`[]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("technitium_settings_proxy_forwarders.this", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("technitium_settings_general.this", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("technitium_settings_recursion.this", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_settings_proxy_forwarders.this", "forwarders.#", "0"),
					resource.TestCheckResourceAttr("technitium_settings_general.this", "dns_server_domain", "ns.settings.test"),
				),
			},
			{
				ResourceName:      "technitium_settings_proxy_forwarders.this",
				ImportState:       true,
				ImportStateId:     "settings_proxy_forwarders",
				ImportStateVerify: true,
			},
		},
	})

	resp, err := api(t).Call(context.Background(), "/api/settings/get", nil)
	require.NoError(t, err)
	assert.Equal(t, "Allow", resp["recursion"], "destroy leaves the settings alone")
	assert.Equal(t, "ns.settings.test", resp["dnsServerDomain"])
}

func TestAccSettingsTables(t *testing.T) {
	config := func(keys string) string {
		return `
resource "technitium_settings_tsig" "this" {
  tsig_keys = [` + keys + `]
}

resource "technitium_settings_general" "this" {
  qpm_prefix_limits_ipv4 = [{ prefix = 24, udp_limit = 6000, tcp_limit = 0 }]
}
`
	}
	zeta := `{ key_name = "zeta", shared_secret = "emV0YS1zZWNyZXQ=", algorithm_name = "hmac-sha256" }`
	alpha := `{ key_name = "alpha", shared_secret = "YWxwaGEtc2VjcmV0", algorithm_name = "hmac-sha512" }`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config(zeta + ", " + alpha),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_settings_tsig.this", "tsig_keys.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("technitium_settings_tsig.this", "tsig_keys.*", map[string]string{"key_name": "alpha", "algorithm_name": "hmac-sha512"}),
					resource.TestCheckResourceAttr("technitium_settings_general.this", "qpm_prefix_limits_ipv4.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs("technitium_settings_general.this", "qpm_prefix_limits_ipv4.*", map[string]string{"prefix": "24", "udp_limit": "6000", "tcp_limit": "0"}),
				),
			},
			{
				Config: config(zeta),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("technitium_settings_tsig.this", plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr("technitium_settings_tsig.this", "tsig_keys.#", "1"),
			},
			{
				ResourceName:      "technitium_settings_tsig.this",
				ImportState:       true,
				ImportStateId:     "settings_tsig",
				ImportStateVerify: true,
			},
			{
				Config: config(""),
				Check:  resource.TestCheckResourceAttr("technitium_settings_tsig.this", "tsig_keys.#", "0"),
			},
		},
	})

	resp, err := api(t).Call(context.Background(), "/api/settings/get", nil)
	require.NoError(t, err)
	assert.Empty(t, resp["tsigKeys"])
}

func TestSpecNamesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range technitium.Specs() {
		assert.False(t, seen[s.Name], "%s is defined twice", s.Name)
		seen[s.Name] = true
	}
}
