package technitium_test

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"
)

func records(t *testing.T, zone, domain, typ string) []map[string]any {
	t.Helper()
	resp := call(t, api(t), "/api/zones/records/get", url.Values{"zone": {zone}, "domain": {domain}})
	var out []map[string]any
	for _, r := range resp["records"].([]any) {
		if rec := r.(map[string]any); rec["type"] == typ {
			out = append(out, rec)
		}
	}
	return out
}

func expectCreate(addr string) resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionCreate)}}
}

func expectUpdate(addr string) resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)}}
}

func TestAccRecordValuesKeepCase(t *testing.T) {
	config := `
resource "technitium_zone" "test" {
  zone = "case.test"
}

resource "technitium_record_txt" "hello" {
  zone   = technitium_zone.test.zone
  domain = "case.test"
  text   = "hello"
}

resource "technitium_record_caa" "issue" {
  zone   = technitium_zone.test.zone
  domain = "case.test"
  flags  = 0
  tag    = "issue"
  value  = "ca.example"
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("case.test"),
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig: func() {
					c := api(t)
					base := url.Values{"zone": {"case.test"}, "domain": {"case.test"}, "type": {"TXT"}}
					call(t, c, "/api/zones/records/update", merge(base, url.Values{"text": {"hello"}, "newText": {"HELLO"}}))
				},
				Config:           config,
				ConfigPlanChecks: expectCreate("technitium_record_txt.hello"),
			},
			{
				PreConfig: func() {
					c := api(t)
					base := url.Values{"zone": {"case.test"}, "domain": {"case.test"}, "type": {"CAA"}, "flags": {"0"}, "tag": {"issue"}}
					call(t, c, "/api/zones/records/update", merge(base, url.Values{"value": {"ca.example"}, "newValue": {"CA.example"}}))
				},
				Config:           config,
				ConfigPlanChecks: expectCreate("technitium_record_caa.issue"),
			},
		},
	})
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

// TestAccNamesWithTrailingDot: the server stores names without the trailing
// dot, which must not fail the apply.
func TestAccNamesWithTrailingDot(t *testing.T) {
	config := `
resource "technitium_zone" "test" {
  zone = "dot.test"
}

resource "technitium_record_cname" "www" {
  zone   = technitium_zone.test.zone
  domain = "www.dot.test"
  cname  = "Target.Example.NET."
}

resource "technitium_record_dname" "old" {
  zone   = technitium_zone.test.zone
  domain = "old.dot.test"
  dname  = "new.example.net."
}

resource "technitium_record_soa" "soa" {
  zone                = technitium_zone.test.zone
  domain              = technitium_zone.test.zone
  primary_name_server = "ns1.dot.test."
  responsible_person  = "hostmaster.dot.test."
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("dot.test"),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_cname.www", "cname", "Target.Example.NET."),
					resource.TestCheckResourceAttr("technitium_record_soa.soa", "primary_name_server", "ns1.dot.test."),
				),
			},
			{
				PreConfig: func() {
					call(t, api(t), "/api/zones/records/update", url.Values{"zone": {"dot.test"}, "domain": {"www.dot.test"}, "type": {"CNAME"}, "cname": {"other.example.net"}})
				},
				Config:           config,
				ConfigPlanChecks: expectUpdate("technitium_record_cname.www"),
			},
		},
	})
}

// TestAccAddressSpelling: an IPv6 address the server writes in another
// spelling still matches, and request-ip-address is refused.
func TestAccAddressSpelling(t *testing.T) {
	config := `
resource "technitium_zone" "test" {
  zone = "ipv6.test"
}

resource "technitium_record_aaaa" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.ipv6.test"
  ip_address = "2001:DB8:0::1"
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("ipv6.test"),
		Steps: []resource.TestStep{
			{
				Config: config + `
resource "technitium_record_a" "me" {
  zone       = technitium_zone.test.zone
  domain     = "me.ipv6.test"
  ip_address = "request-ip-address"
}
`,
				ExpectError: regexp.MustCompile(`must be an IP address`),
			},
			{Config: config},
			{
				ResourceName:      "technitium_record_aaaa.www",
				ImportState:       true,
				ImportStateId:     "ipv6.test/www.ipv6.test/2001:DB8:0::1",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccURIRecordNormalized(t *testing.T) {
	config := `
resource "technitium_zone" "test" {
  zone = "uri.test"
}

resource "technitium_record_uri" "web" {
  zone         = technitium_zone.test.zone
  domain       = "_http._tcp.uri.test"
  uri_priority = 10
  uri_weight   = 1
  uri          = "HTTPS://Example.com"
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("uri.test"),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("technitium_record_uri.web", "uri", "HTTPS://Example.com"),
			},
			{
				ResourceName:      "technitium_record_uri.web",
				ImportState:       true,
				ImportStateId:     "uri.test/_http._tcp.uri.test/10/1/HTTPS://Example.com",
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccIDN: the server returns internationalized names as punycode.
func TestAccIDN(t *testing.T) {
	config := `
resource "technitium_zone" "test" {
  zone = "bücher.test"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.bücher.test"
  ip_address = "10.0.0.1"
}

resource "technitium_record_cname" "shop" {
  zone   = technitium_zone.test.zone
  domain = "shop.bücher.test"
  cname  = "www.bücher.test"
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("xn--bcher-kva.test"),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("technitium_record_a.www", "domain", "www.bücher.test"),
			},
			{
				ResourceName:      "technitium_record_a.www",
				ImportState:       true,
				ImportStateId:     "bücher.test/www.bücher.test/10.0.0.1",
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccServiceBindingAutoHints: the server adds the hints to svcParams.
func TestAccServiceBindingAutoHints(t *testing.T) {
	config := func(ttl int, auto bool) string {
		return fmt.Sprintf(`
resource "technitium_zone" "test" {
  zone = "hints.test"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.hints.test"
  ip_address = "10.0.0.1"
}

resource "technitium_record_https" "www" {
  zone            = technitium_zone.test.zone
  domain          = "www.hints.test"
  svc_priority    = 1
  svc_target_name = "www.hints.test"
  svc_params      = { alpn = "h2" }
  auto_ipv4_hint  = %t
  ttl             = %d
  depends_on      = [technitium_record_a.www]
}
`, auto, ttl)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("hints.test"),
		Steps: []resource.TestStep{
			{
				Config: config(300, true),
				Check: func(*terraform.State) error {
					recs := records(t, "hints.test", "www.hints.test", "HTTPS")
					if len(recs) != 1 {
						return fmt.Errorf("%d HTTPS records", len(recs))
					}
					if _, ok := recs[0]["rData"].(map[string]any)["svcParams"].(map[string]any)["ipv4hint"]; !ok {
						return fmt.Errorf("no ipv4hint: %v", recs[0]["rData"])
					}
					return nil
				},
			},
			{Config: config(600, true), ConfigPlanChecks: expectUpdate("technitium_record_https.www")},
			{
				ResourceName:      "technitium_record_https.www",
				ImportState:       true,
				ImportStateId:     `{"zone": "hints.test", "domain": "www.hints.test", "svc_priority": 1, "svc_target_name": "www.hints.test", "svc_params": {"alpn": "h2"}}`,
				ImportStateVerify: true,
			},
			{Config: config(600, false), ConfigPlanChecks: expectUpdate("technitium_record_https.www")},
			{Config: config(600, true), ConfigPlanChecks: expectUpdate("technitium_record_https.www")},
			{
				Config: `
resource "technitium_zone" "test" {
  zone = "hints.test"
}
`,
				Check: func(*terraform.State) error {
					if recs := records(t, "hints.test", "www.hints.test", "HTTPS"); len(recs) != 0 {
						return fmt.Errorf("HTTPS record left behind: %v", recs)
					}
					return nil
				},
			},
		},
	})
}

// TestAccDSAlgorithmSpelling uses the names the server writes with underscores.
func TestAccDSAlgorithmSpelling(t *testing.T) {
	config := `
resource "technitium_zone" "test" {
  zone = "ds.test"
}

resource "technitium_record_ds" "nsec3" {
  zone        = technitium_zone.test.zone
  domain      = "child.ds.test"
  key_tag     = 1
  algorithm   = "RSASHA1_NSEC3_SHA1"
  digest_type = "SHA1"
  digest      = "2BB183AF5F22588179A53B0A98631FAD1A292118"
}

resource "technitium_record_ds" "gost" {
  zone        = technitium_zone.test.zone
  domain      = "child.ds.test"
  key_tag     = 2
  algorithm   = "ECC_GOST"
  digest_type = "GOST_R_34_11_94"
  digest      = "2BB183AF5F22588179A53B0A98631FAD1A292118D3F04E8B8F6A1FB4A10A8D0D"
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("ds.test"),
		Steps: []resource.TestStep{
			{Config: config},
			{ResourceName: "technitium_record_ds.gost", ImportState: true, ImportStateVerify: true},
		},
	})
}

// TestAccFWDRecordHasNoTTL: the server keeps every FWD record at TTL 0.
func TestAccFWDRecordHasNoTTL(t *testing.T) {
	config := func(ttl string) string {
		return `
resource "technitium_zone" "test" {
  zone = "fwdttl.test"
  type = "Forwarder"
}

resource "technitium_record_fwd" "test" {
  zone      = technitium_zone.test.zone
  domain    = "sub.fwdttl.test"
  forwarder = "192.0.2.53"
  ` + ttl + `
}
`
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("fwdttl.test"),
		Steps: []resource.TestStep{
			{Config: config("ttl = 300"), ExpectError: regexp.MustCompile(`Unsupported argument`)},
			{Config: config("")},
			{ResourceName: "technitium_record_fwd.test", ImportState: true, ImportStateVerify: true},
		},
	})
}

func TestAccRecordDefaults(t *testing.T) {
	config := func(extra string) string {
		return `
resource "technitium_zone" "test" {
  zone = "defaults.test"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.defaults.test"
  ip_address = "10.0.0.1"
  ` + extra + `
}
`
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("defaults.test"),
		Steps: []resource.TestStep{
			{Config: config("disable = true\n  comments = \"web\"\n  expiry_ttl = 86400")},
			{
				Config:           config(""),
				ConfigPlanChecks: expectUpdate("technitium_record_a.www"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record_a.www", "disable", "false"),
					resource.TestCheckResourceAttr("technitium_record_a.www", "comments", ""),
					resource.TestCheckResourceAttr("technitium_record_a.www", "expiry_ttl", "0"),
				),
			},
			{Config: config("comments = \"web\"")},
			{
				PreConfig: func() {
					call(t, api(t), "/api/zones/records/update", url.Values{"zone": {"defaults.test"}, "domain": {"www.defaults.test"}, "type": {"A"}, "ipAddress": {"10.0.0.1"}, "comments": {""}})
				},
				Config:           config("comments = \"web\""),
				ConfigPlanChecks: expectUpdate("technitium_record_a.www"),
			},
			{ResourceName: "technitium_record_a.www", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"ptr", "create_ptr_zone"}},
		},
	})
}

func TestAccAddressPTRKeepsOthers(t *testing.T) {
	config := func(ttl int, ptr bool) string {
		return fmt.Sprintf(`
resource "technitium_zone" "test" {
  zone = "ptr.test"
}

resource "technitium_zone" "reverse" {
  zone = "100.51.198.in-addr.arpa"
}

resource "technitium_record_a" "www" {
  zone       = technitium_zone.test.zone
  domain     = "www.ptr.test"
  ip_address = "198.51.100.7"
  ptr        = %t
  ttl        = %d
  depends_on = [technitium_zone.reverse]
}

resource "technitium_record_ptr" "other" {
  zone       = technitium_zone.reverse.zone
  domain     = "7.100.51.198.in-addr.arpa"
  ptr_name   = "other.ptr.test"
  depends_on = [technitium_record_a.www]
}
`, ptr, ttl)
	}
	ptrs := func(want int) resource.TestCheckFunc {
		return func(*terraform.State) error {
			if got := records(t, "100.51.198.in-addr.arpa", "7.100.51.198.in-addr.arpa", "PTR"); len(got) != want {
				return fmt.Errorf("want %d PTR records, got %v", want, got)
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("ptr.test", "100.51.198.in-addr.arpa"),
		Steps: []resource.TestStep{
			{Config: config(300, true), Check: ptrs(2)},
			{Config: config(600, true), ConfigPlanChecks: expectUpdate("technitium_record_a.www"), Check: ptrs(2)},
			{Config: config(600, false), Check: ptrs(2)},
		},
	})
}

func TestAccAPPRecordOnePerName(t *testing.T) {
	config := `
resource "technitium_zone" "test" {
  zone = "appone.test"
}

resource "technitium_record_app" "a" {
  zone       = technitium_zone.test.zone
  domain     = "www.appone.test"
  app_name   = "Missing App"
  class_path = "Missing.A"
  record_data = jsonencode({})
}

resource "technitium_record_app" "b" {
  zone       = technitium_zone.test.zone
  domain     = "www.appone.test"
  app_name   = "Missing App"
  class_path = "Missing.B"
  record_data = jsonencode({})
  depends_on = [technitium_record_app.a]
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             zonesDestroyed("appone.test"),
		Steps: []resource.TestStep{
			{Config: config, ExpectError: regexp.MustCompile(`already exists`)},
		},
	})
}

func TestAccDHCPScopeExists(t *testing.T) {
	acceptance(t)
	c := api(t)
	call(t, c, "/api/dhcp/scopes/set", url.Values{"name": {"taken"}, "startingAddress": {"192.168.60.10"}, "endingAddress": {"192.168.60.20"}, "subnetMask": {"255.255.255.0"}})
	t.Cleanup(func() {
		_, err := c.Call(context.Background(), "/api/dhcp/scopes/delete", url.Values{"name": {"taken"}})
		require.NoError(t, err)
	})
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "technitium_dhcp_scope" "taken" {
  name             = "taken"
  starting_address = "192.168.60.100"
  ending_address   = "192.168.60.200"
  subnet_mask      = "255.255.255.0"
}
`,
				ExpectError: regexp.MustCompile(`already exists`),
			},
		},
	})
}
