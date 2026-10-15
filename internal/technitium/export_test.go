package technitium_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/terraforming"
	"github.com/ilijamt/terraform-provider-technitium/internal/testserver"
)

func TestAccExport(t *testing.T) {
	acceptance(t)
	resetSSO(t)
	resetLDAP(t)
	c := api(t)
	// The app needs the DNS App Store; without it the export runs without.
	// It goes in before the SSO change below restarts the web service.
	app := tryStoreURL(t, "Split Horizon")
	if app != "" {
		call(t, c, "/api/apps/downloadAndInstall", url.Values{"name": {"Split Horizon"}, "url": {app}})
		t.Cleanup(func() {
			_, _ = c.Call(context.Background(), "/api/apps/uninstall", url.Values{"name": {"Split Horizon"}})
		})
	}
	for _, step := range []struct {
		path   string
		params url.Values
	}{
		{"/api/zones/create", url.Values{"zone": {"export.test"}}},
		{"/api/zones/records/add", url.Values{"zone": {"export.test"}, "domain": {"www.export.test"}, "type": {"A"}, "ipAddress": {"10.0.0.1"}}},
		{"/api/zones/records/add", url.Values{"zone": {"export.test"}, "domain": {"www.export.test"}, "type": {"A"}, "ipAddress": {"10.0.0.2"}}},
		{"/api/zones/records/add", url.Values{"zone": {"export.test"}, "domain": {"export.test"}, "type": {"TXT"}, "text": {"v=spf1 ip4:10.0.0.0/8 -all"}}},
		{"/api/zones/records/add", url.Values{"zone": {"export.test"}, "domain": {"export.test"}, "type": {"MX"}, "exchange": {"mail.export.test"}, "preference": {"10"}}},
		{"/api/zones/records/add", url.Values{"zone": {"export.test"}, "domain": {"_dns.export.test"}, "type": {"SVCB"}, "svcPriority": {"1"}, "svcTargetName": {"dns.export.test"}, "svcParams": {"alpn|h2"}}},
		{"/api/zones/dnssec/sign", url.Values{"zone": {"export.test"}, "algorithm": {"ECDSA"}, "curve": {"P256"}}},
		{"/api/zones/create", url.Values{"zone": {"fwd.export.test"}, "type": {"Forwarder"}, "initializeForwarder": {"false"}}},
		{"/api/zones/records/add", url.Values{"zone": {"fwd.export.test"}, "domain": {"fwd.export.test"}, "type": {"FWD"}, "protocol": {"Udp"}, "forwarder": {"192.0.2.53"}}},
		{"/api/dhcp/scopes/set", url.Values{"name": {"export"}, "startingAddress": {"192.168.70.10"}, "endingAddress": {"192.168.70.200"}, "subnetMask": {"255.255.255.0"}, "exclusions": {"192.168.70.10|192.168.70.20"}}},
		{"/api/dhcp/scopes/addReservedLease", url.Values{"name": {"export"}, "hardwareAddress": {"00-11-22-AA-BB-CC"}, "ipAddress": {"192.168.70.50"}, "hostName": {"nas"}}},
		{"/api/admin/users/create", url.Values{"user": {"exporter"}, "pass": {"exporter-pass"}}},
		{"/api/admin/groups/create", url.Values{"group": {"Exporters"}, "description": {"export test"}}},
		{"/api/admin/groups/set", url.Values{"group": {"Exporters"}, "members": {"exporter"}}},
		{"/api/blocked/add", url.Values{"domain": {"ads.export.test"}}},
		{"/api/allowed/add", url.Values{"domain": {"cdn.export.test"}}},
		{"/api/settings/set", url.Values{"tsigKeys": {"export-key|c2VjcmV0LWtleQ==|hmac-sha256"}}},
		{"/api/admin/ldap/set", url.Values{"ldapServer": {"ldap.example.com"}, "ldapBindUsername": {"CN=svc,DC=example,DC=com"}, "ldapBindPassword": {"s3cret"}, "ldapGroupMap": {"CN=dns,DC=example,DC=com|DNS Administrators"}}},
		{"/api/admin/sso/set", url.Values{"ssoClientId": {"technitium"}, "ssoClientSecret": {"s3cret"}, "ssoAuthority": {"https://idp.example.com/realms/home"}, "ssoGroupMap": {"dns-admins|DNS Administrators"}}},
	} {
		call(t, c, step.path, step.params)
	}

	out := t.TempDir()
	require.NoError(t, terraforming.Run(context.Background(), terraforming.Options{
		Host: os.Getenv("TECHNITIUM_HOST"), Username: testserver.Username, Password: testserver.Password,
		Out: out, Version: "test",
	}))
	generated := readConfig(t, out)
	for _, want := range []string{
		`resource "technitium_zone" "export_test"`,
		`resource "technitium_record_soa" "export_test"`,
		`resource "technitium_record_a" "www_export_test_2"`,
		`resource "technitium_record_txt" "export_test"`,
		`resource "technitium_record_svcb" "dns_export_test"`,
		`resource "technitium_record_fwd" "fwd_export_test"`,
		`resource "technitium_dhcp_scope" "export"`,
		`resource "technitium_dhcp_reserved_lease" "export_nas"`,
		`resource "technitium_user" "exporter"`,
		`resource "technitium_group" "exporters"`,
		`resource "technitium_group_members" "exporters"`,
		`resource "technitium_group_members" "administrators"`,
		`prevent_destroy = true`,
		`data "technitium_group" "administrators"`,
		`user_permissions = []`,
		`resource "technitium_settings_sso" "this"`,
		`resource "technitium_zone_permissions" "export_test"`,
		`resource "technitium_zone_dnssec" "export_test"`,
		"# Administrators and DNS Administrators always have full rights here: the server adds them, so they are not listed.\nresource \"technitium_permissions_zones\" \"this\"",
		"# Administrators always has full rights here: the server adds it, so it is not listed.\nresource \"technitium_permissions_logs\" \"this\"",
		`data "technitium_group" "everyone"`,
		`group   = data.technitium_group.administrators.name`,
		`resource "technitium_permissions_zones" "this"`,
		`resource "technitium_blocked_zone" "ads_export_test"`,
		`resource "technitium_allowed_zone" "cdn_export_test"`,
		`resource "technitium_settings_tsig" "this"`,
		`shared_secret  = var.settings_tsig_this_tsig_keys_0_shared_secret`,
	} {
		require.Contains(t, generated, want)
	}
	require.NotContains(t, generated, `resource "technitium_group" "administrators"`, "built-in groups are not exported")
	require.NotRegexp(t, `name\s+= data\.technitium_group\.administrators\.name`, generated, "kept groups are left out of permissions")
	for _, want := range []string{
		`zone\s+= technitium_zone\.export_test\.zone`,
		`scope\s+= technitium_dhcp_scope\.export\.name`,
		`members\s+= \[technitium_user\.exporter\.username\]`,
		`group\s+= technitium_group\.exporters\.name`,
		`id = "export\.test/www\.export\.test/10\.0\.0\.1"`,
		`local_group\s+= data\.technitium_group\.dns_administrators\.name`,
		`client_secret\s+= var\.settings_sso_this_client_secret`,
		`bind_password\s+= var\.settings_ldap_this_bind_password`,
		`id = "\{\\"`,
	} {
		require.Regexp(t, want, generated)
	}
	if app != "" {
		require.Contains(t, generated, `resource "technitium_app" "split_horizon"`)
		require.NotContains(t, generated, app, "an imported app is adopted without a URL")
		require.Regexp(t, `name\s+= technitium_app\.split_horizon\.name`, generated)
		require.Contains(t, generated, "config = jsonencode({", "app config as HCL")
	}
	_, err := os.Stat(filepath.Join(out, "records_export_test.tf"))
	require.NoError(t, err, "records are written per zone")
	_, err = os.Stat(filepath.Join(out, "technitium_versions.tf"))
	require.NoError(t, err, "the provider source is in a file a root module is unlikely to have")
	_, err = os.Stat(filepath.Join(out, "versions.tf"))
	require.ErrorIs(t, err, os.ErrNotExist)

	// Destroying the admin user or a built-in group would break the server,
	// so the last step drops them from state first.
	// Exported users are protected from destroy, so they are dropped from
	// state at the end and the test user is removed directly.
	keep := []string{"technitium_user.admin", "technitium_user.exporter", "technitium_dhcp_scope.default"}
	t.Cleanup(func() {
		_, _ = c.Call(context.Background(), "/api/admin/users/delete", url.Values{"user": {"exporter"}})
	})
	vars := config.Variables{
		"user_admin_password":                          config.StringVariable(testserver.Password),
		"user_exporter_password":                       config.StringVariable("exporter-pass"),
		"settings_tsig_this_tsig_keys_0_shared_secret": config.StringVariable("c2VjcmV0LWtleQ=="),
		"settings_sso_this_client_secret":              config.StringVariable("s3cret"),
		"settings_ldap_this_bind_password":             config.StringVariable("s3cret"),
	}
	t.Setenv("TF_ACC_PROVIDER_NAMESPACE", "ilijamt")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:          generated,
				ConfigVariables: vars,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{onlyImports{}},
				},
			},
			{
				Config:          withoutResources(t, generated, keep),
				ConfigVariables: vars,
			},
		},
	})
}

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

func withoutResources(t *testing.T, cfg string, addrs []string) string {
	f, diags := hclwrite.ParseConfig([]byte(cfg), "generated.tf", hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())
	body := f.Body()
	values := map[string]string{}
	for _, b := range body.Blocks() {
		var addr string
		switch b.Type() {
		case "resource":
			addr = strings.Join(b.Labels(), ".")
		case "import":
			addr = strings.TrimSpace(string(b.Body().GetAttribute("to").Expr().BuildTokens(nil).Bytes()))
		}
		if !slices.Contains(addrs, addr) {
			continue
		}
		if b.Type() == "resource" {
			for name, a := range b.Body().Attributes() {
				values[addr+"."+name] = strings.TrimSpace(string(a.Expr().BuildTokens(nil).Bytes()))
			}
		}
		body.RemoveBlock(b)
	}
	for _, addr := range addrs {
		r := body.AppendNewBlock("removed", nil).Body()
		typ, name, _ := strings.Cut(addr, ".")
		r.SetAttributeTraversal("from", hcl.Traversal{hcl.TraverseRoot{Name: typ}, hcl.TraverseAttr{Name: name}})
		r.AppendNewBlock("lifecycle", nil).Body().SetAttributeRaw("destroy", hclwrite.TokensForIdentifier("false"))
	}
	out := string(f.Bytes())
	for ref, value := range values {
		out = strings.ReplaceAll(out, ref, value)
	}
	return out
}

// onlyImports fails a plan that changes anything but values the server never
// returns: passwords, the SSO client secret and app URLs.
type onlyImports struct{}

func (onlyImports) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	imported := 0
	for _, rc := range req.Plan.ResourceChanges {
		if rc.Change.Importing != nil {
			imported++
		}
		switch {
		case rc.Change.Actions.NoOp():
		case rc.Change.Actions.Update() && (rc.Type == "technitium_user" || rc.Type == "technitium_settings_sso" || rc.Type == "technitium_settings_ldap" || rc.Type == "technitium_app"):
		default:
			resp.Error = fmt.Errorf("%s: unexpected %v", rc.Address, rc.Change.Actions)
			return
		}
	}
	if imported == 0 {
		resp.Error = fmt.Errorf("nothing imported")
	}
}
