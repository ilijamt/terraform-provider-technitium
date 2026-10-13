package technitium_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
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

// ownServer is set when TestMain runs the test server in Docker, so other
// containers can reach it.
var ownServer bool

// TestMain starts a Technitium container for the acceptance tests (TF_ACC=1).
func TestMain(m *testing.M) {
	if os.Getenv("TF_ACC") == "" {
		os.Exit(m.Run())
	}
	ownServer = os.Getenv("TECHNITIUM_HOST") == ""
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
	code := m.Run()
	stop()
	os.Exit(code)
}

var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"technitium": providerserver.NewProtocol6WithError(provider.New("test", nil, technitium.Resources())()),
}

func TestSchemas(t *testing.T) {
	srv, err := factories["technitium"]()
	require.NoError(t, err)
	resp, err := srv.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	require.NoError(t, err)
	for _, d := range resp.Diagnostics {
		t.Errorf("%s: %s", d.Summary, d.Detail)
	}
	assert.Len(t, resp.ResourceSchemas, len(technitium.Specs()))
	for _, name := range []string{"technitium_settings_general", "technitium_zone", "technitium_record_a", "technitium_record_fwd"} {
		assert.Contains(t, resp.ResourceSchemas, name)
	}
}

// api logs in to the test server, for changes made behind Terraform's back.
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

// acceptance skips a test that sets up the server before resource.Test, which
// skips on its own, when acceptance tests are off.
func acceptance(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run acceptance tests")
	}
}

// call makes an API call for setting up a test, failing it on an error.
func call(t *testing.T, c *client.Client, path string, params url.Values) map[string]any {
	t.Helper()
	resp, err := c.Call(context.Background(), path, params)
	require.NoError(t, err, path)
	return resp
}

// zonesDestroyed checks that destroy removed the zones from the server.
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
				// Only comments change: an in-place update that keeps ttl and disable.
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

// The udm setup: a forwarder zone for reverse lookups pointing at the router.
// The zone starts empty, so the record is its only forwarder.
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
				// Destroying the resource leaves the record as it is.
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

// recordTypes holds a record of each type not covered elsewhere, by
// resource type. APP needs an installed DNS app, so it is left out.
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
				// A change made behind Terraform's back shows up as drift.
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

// TestAccSecondaryZones transfers zones from a second server, which creating
// a Secondary, SecondaryForwarder or Stub zone needs.
func TestAccSecondaryZones(t *testing.T) {
	acceptance(t)
	if !ownServer {
		t.Skip("needs the test server in Docker to reach a second server; TECHNITIUM_HOST is set")
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
				// The transfer protocol changes in place.
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
				// A change made behind Terraform's back shows up as drift.
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
				// One area changes; the others are left alone.
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
				// Out of the server's sorted order, which a set ignores.
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
