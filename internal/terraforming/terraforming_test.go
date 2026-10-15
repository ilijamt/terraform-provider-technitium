package terraforming

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"testing"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
)

func TestLabels(t *testing.T) {
	l := labels{}
	assert.Equal(t, "www_example_com", l.next("www.example.com"))
	assert.Equal(t, "www_example_com_2", l.next("WWW.example.com"))
	assert.Equal(t, "r_178_168_192_in_addr_arpa", l.next("178.168.192.in-addr.arpa"))
	assert.Equal(t, "dns_example_com", l.next("_dns.example.com"))
	assert.Equal(t, "wildcard_example_com", l.next("*.example.com"))
	assert.Equal(t, "ingress_tpi_example_com", l.next("ingress-tpi.example.com"))
	assert.Equal(t, "r_", l.next(""))
}

func TestFilesAndOrder(t *testing.T) {
	objs := []object{{Type: "zone"}, {Type: "record_a"}, {Type: "record_soa"}, {Type: "settings_tsig"}}
	assert.Equal(t, objs, filter(objs, nil))
	assert.Equal(t, []object{{Type: "record_a"}, {Type: "record_soa"}}, filter(objs, []string{"record_*"}))
	assert.Equal(t, []object{{Type: "zone"}}, filter(objs, []string{"zone"}))

	assert.Equal(t, "zones.tf", fileFor(object{Type: "zone", Zone: "example.com"}))
	assert.Equal(t, "zones.tf", fileFor(object{Type: "record_soa", Zone: "example.com"}))
	assert.Equal(t, "records_example_com.tf", fileFor(object{Type: "record_https", Zone: "example.com"}))
	assert.Equal(t, "settings.tf", fileFor(object{Type: "settings_general"}))
	assert.Equal(t, "access.tf", fileFor(object{Type: "permissions_zones"}))

	zones := []object{
		{Type: "record_soa", Zone: "b.com", name: "b_com"},
		{Type: "zone", Zone: "b.com", name: "b_com"},
		{Type: "record_soa", Zone: "a.com", name: "a_com"},
		{Type: "zone", Zone: "a.com", name: "a_com"},
	}
	sort.SliceStable(zones, func(i, j int) bool { return before(zones[i], zones[j]) })
	var got []string
	for _, z := range zones {
		got = append(got, z.Type+" "+z.Zone)
	}
	assert.Equal(t, []string{"zone a.com", "record_soa a.com", "zone b.com", "record_soa b.com"}, got, "each zone, then its SOA")
}

func TestRecordID(t *testing.T) {
	spec := framework.Record("MX",
		framework.Attr{Name: "exchange", Param: "exchange", Type: framework.String, Key: "rData.exchange", Identity: true},
		framework.Attr{Name: "preference", Param: "preference", Type: framework.Int64, Key: "rData.preference", Identity: true},
	)
	record := func(exchange string) map[string]any {
		return map[string]any{"name": "example.com", "rData": map[string]any{"exchange": exchange, "preference": json.Number("10")}}
	}
	id, err := recordID(spec, "example.com", record("mail.example.com"))
	require.NoError(t, err)
	assert.Equal(t, "example.com/example.com/mail.example.com/10", id, "the short form when unambiguous")

	id, err = recordID(spec, "example.com", record("a/b"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"zone":"example.com","domain":"example.com","exchange":"a/b","preference":10}`, id, "JSON for a value with /")

	id, err = recordID(spec, "example.com", record(""))
	require.NoError(t, err)
	assert.JSONEq(t, `{"zone":"example.com","domain":"example.com","exchange":"","preference":10}`, id, "JSON for an empty value")
}

func TestReferences(t *testing.T) {
	refs := references([]object{
		{Type: "zone", ID: "Example.com", name: "example_com"},
		{Type: "user", ID: "alice", name: "alice"},
		{Type: "user", ID: "sso@example.com", name: "sso_example_com", Data: true},
		{Type: "group", ID: "Administrators", name: "administrators", Data: true},
		{Type: "group", ID: "Ops", name: "ops"},
		{Type: "dhcp_scope", ID: "lan", name: "lan"},
	})
	got := func(typ string, path []string, s string) string {
		t, ok := refs(typ)(path, s)
		if !ok {
			return "-"
		}
		return string(t.Bytes())
	}
	assert.Equal(t, "technitium_zone.example_com.zone", got("record_a", []string{"zone"}, "example.com"))
	assert.Equal(t, "technitium_dhcp_scope.lan.name", got("dhcp_reserved_lease", []string{"scope"}, "lan"))
	assert.Equal(t, "data.technitium_group.administrators.name", got("group_members", []string{"group"}, "Administrators"))
	assert.Equal(t, "technitium_user.alice.username", got("group_members", []string{"members", "0"}, "alice"))
	assert.Equal(t, "technitium_group.ops.name", got("permissions_zones", []string{"group_permissions", "1", "name"}, "Ops"))
	assert.Equal(t, "technitium_user.alice.username", got("permissions_zones", []string{"user_permissions", "0", "username"}, "alice"))
	assert.Equal(t, "data.technitium_user.sso_example_com.username", got("group_members", []string{"members", "1"}, "sso@example.com"))
	assert.Equal(t, "-", got("record_a", []string{"domain"}, "example.com"), "only the zone is referred to")
	assert.Equal(t, "-", got("group_members", []string{"members", "0"}, "bob"), "not exported")
}

func TestResourceBlockSecrets(t *testing.T) {
	row := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"key_name": tftypes.String, "shared_secret": tftypes.String}}
	schema := &tfprotov6.Schema{Block: &tfprotov6.SchemaBlock{Attributes: []*tfprotov6.SchemaAttribute{
		{Name: "id", Type: tftypes.String, Computed: true},
		{Name: "username", Type: tftypes.String, Required: true},
		{Name: "password", Type: tftypes.String, Required: true, Sensitive: true},
		{Name: "serial", Type: tftypes.Number, Computed: true},
		{Name: "comments", Type: tftypes.String, Optional: true, Computed: true},
		{Name: "keys", NestedType: &tfprotov6.SchemaObject{Nesting: tfprotov6.SchemaObjectNestingModeSet, Attributes: []*tfprotov6.SchemaAttribute{
			{Name: "key_name", Type: tftypes.String, Required: true},
			{Name: "shared_secret", Type: tftypes.String, Required: true, Sensitive: true},
		}}, Optional: true},
	}}}
	state := map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, "alice"),
		"username": tftypes.NewValue(tftypes.String, "alice"),
		"password": tftypes.NewValue(tftypes.String, nil),
		"serial":   tftypes.NewValue(tftypes.Number, 7),
		"comments": tftypes.NewValue(tftypes.String, ""),
		"keys": tftypes.NewValue(tftypes.Set{ElementType: row}, []tftypes.Value{tftypes.NewValue(row, map[string]tftypes.Value{
			"key_name": tftypes.NewValue(tftypes.String, "xfer"), "shared_secret": tftypes.NewValue(tftypes.String, "c2VjcmV0"),
		})}),
	}

	f := hclwrite.NewEmptyFile()
	vars := resourceBlock(f.Body(), "user", "alice", schema, state, nil, nil)
	got := string(hclwrite.Format(f.Bytes()))

	assert.Contains(t, got, `username = "alice"`)
	assert.Contains(t, got, `password = var.user_alice_password`, "a required secret the server does not return")
	assert.Contains(t, got, `shared_secret = var.user_alice_keys_0_shared_secret`)
	assert.NotContains(t, got, "c2VjcmV0", "no secret is written")
	assert.NotContains(t, got, "serial", "read-only")
	assert.NotContains(t, got, "comments", "empty")
	assert.NotContains(t, got, " id ", "the id is not configuration")
	assert.Len(t, vars, 2)
}

type fakeAPI map[string]map[string]any

func (f fakeAPI) Call(_ context.Context, path string, _ url.Values) (map[string]any, error) {
	return f[path], nil
}

func TestListReadsSSOUsers(t *testing.T) {
	var warnings []string
	l := &lister{api: fakeAPI{
		"/api/admin/users/list": {"users": []any{
			map[string]any{"username": "admin", "isSsoUser": false},
			map[string]any{"username": "someone@example.com", "isSsoUser": true},
		}},
		"/api/admin/groups/list":      {"groups": []any{}},
		"/api/admin/permissions/list": {"permissions": []any{}},
	}, warn: func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }}

	objs, err := l.listAccess(context.Background())
	require.NoError(t, err)
	var users, data []string
	for _, o := range objs {
		switch {
		case o.Type == "user" && o.Data:
			data = append(data, o.ID)
		case o.Type == "user":
			users = append(users, o.ID)
		}
	}
	assert.Equal(t, []string{"admin"}, users)
	assert.Equal(t, []string{"someone@example.com"}, data, "an SSO user is a data source")
	assert.Empty(t, warnings)
}

func TestSSOClientSecret(t *testing.T) {
	schema := &tfprotov6.Schema{Block: &tfprotov6.SchemaBlock{Attributes: []*tfprotov6.SchemaAttribute{
		{Name: "client_id", Type: tftypes.String, Optional: true, Computed: true},
		{Name: "client_secret", Type: tftypes.String, Optional: true, Sensitive: true},
	}}}
	render := func(clientID string) string {
		f := hclwrite.NewEmptyFile()
		resourceBlock(f.Body(), "settings_sso", "this", schema, map[string]tftypes.Value{
			"client_id":     tftypes.NewValue(tftypes.String, clientID),
			"client_secret": tftypes.NewValue(tftypes.String, nil),
		}, nil, nil)
		return string(f.Bytes())
	}
	assert.Contains(t, render("technitium"), "var.settings_sso_this_client_secret", "a configured client needs its secret")
	assert.NotContains(t, render(""), "client_secret", "no client, no secret")
}

func TestJSONEncode(t *testing.T) {
	got, ok := jsonencode("{\r\n  \"enableLogging\": true,\r\n  \"maxLogDays\": 7,\r\n  \"path\": \"q.db\",\r\n  \"networks\": { \"lan\": [\"10.0.0.0/8\"] }\r\n}")
	require.True(t, ok)
	f := hclwrite.NewEmptyFile()
	f.Body().SetAttributeRaw("config", got)
	assert.Equal(t, `config = jsonencode({
  enableLogging = true
  maxLogDays    = 7
  networks = {
    lan = ["10.0.0.0/8"]
  }
  path = "q.db"
})
`, string(hclwrite.Format(f.Bytes())))

	_, ok = jsonencode("export.test/www.export.test/10.0.0.1")
	assert.False(t, ok, "not JSON")
	_, ok = jsonencode(`"just a string"`)
	assert.False(t, ok, "only objects and arrays")
}

func TestKeptNote(t *testing.T) {
	note := func(kept ...string) string {
		return keptNote(framework.Spec{Attrs: []framework.Attr{{Name: "group_permissions", Kept: kept}}})
	}
	assert.Equal(t, "# Administrators always has full rights here: the server adds it, so it is not listed.\n", note("Administrators"))
	assert.Equal(t, "# Administrators and DNS Administrators always have full rights here: the server adds them, so they are not listed.\n",
		note("Administrators", "DNS Administrators"))
	assert.Empty(t, keptNote(framework.Spec{Attrs: []framework.Attr{{Name: "zone"}}}))
}
