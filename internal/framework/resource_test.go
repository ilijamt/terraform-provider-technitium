package framework

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
)

// mockAPI records every call and answers with respond.
type mockAPI struct {
	calls   []call
	respond func(path string, params url.Values) (map[string]any, error)
}

type call struct {
	path   string
	params url.Values
}

func (m *mockAPI) Call(_ context.Context, path string, params url.Values) (map[string]any, error) {
	m.calls = append(m.calls, call{path, params})
	if m.respond == nil {
		return map[string]any{}, nil
	}
	return m.respond(path, params)
}

func (m *mockAPI) paths() []string {
	var out []string
	for _, c := range m.calls {
		out = append(out, c.path)
	}
	return out
}

var recordA = Spec{
	Name: "record_a", Create: "add", Read: "get", Update: "update", Delete: "delete",
	List: "records", Fixed: map[string]string{"type": "A"},
	Attrs: []Attr{
		{Name: "zone", Param: "zone", Type: String, Required: true, Identity: true, Create: true, Update: true},
		{Name: "domain", Param: "domain", Type: String, Key: "name", Required: true, Identity: true, Create: true, Update: true},
		{Name: "ip_address", Param: "ipAddress", Type: String, Key: "rData.ipAddress", Required: true, Identity: true, Create: true, Update: true},
		{Name: "ttl", Param: "ttl", Type: Int64, Key: "ttl", Create: true, Update: true},
		{Name: "comments", Param: "comments", Type: String, Key: "comments", Create: true, Update: true},
		{Name: "disable", Param: "disable", Type: Bool, Key: "disabled", Update: true},
		{Name: "ptr", Param: "ptr", Type: Bool, Create: true},
	},
}

func str(s string) tftypes.Value   { return tftypes.NewValue(tftypes.String, s) }
func num(i int64) tftypes.Value    { return tftypes.NewValue(tftypes.Number, new(big.Float).SetInt64(i)) }
func boolean(b bool) tftypes.Value { return tftypes.NewValue(tftypes.Bool, b) }
func unknown(t Type) tftypes.Value { return tftypes.NewValue(t.tf(), tftypes.UnknownValue) }

func record(ip string, ttl int64) map[string]any {
	return map[string]any{"name": "www.example.com", "type": "A", "ttl": json.Number(strconv.FormatInt(ttl, 10)), "disabled": false, "rData": map[string]any{"ipAddress": ip}}
}

func identity() map[string]tftypes.Value {
	return map[string]tftypes.Value{"zone": str("example.com"), "domain": str("www.example.com"), "ip_address": str("10.0.0.2")}
}

func TestReadPicksMatchingRecord(t *testing.T) {
	api := &mockAPI{respond: func(string, url.Values) (map[string]any, error) {
		other := record("10.0.0.2", 60)
		other["type"] = "AAAA"
		return map[string]any{"records": []any{record("10.0.0.1", 60), other, record("10.0.0.2", 300)}}, nil
	}}
	r := &Resource{spec: recordA, client: api}

	prior := identity()
	prior["ptr"] = boolean(true)
	state, err := r.read(context.Background(), prior)
	require.NoError(t, err)

	assert.Equal(t, num(300), state["ttl"])
	assert.Equal(t, boolean(false), state["disable"])
	assert.Equal(t, boolean(true), state["ptr"], "values the server does not return are kept")
	assert.True(t, state["comments"].IsNull(), "missing optional values are null")
	assert.Equal(t, str("example.com/www.example.com/10.0.0.2"), state["id"])
	assert.Equal(t, "A", api.calls[0].params.Get("type"))
	assert.Equal(t, "www.example.com", api.calls[0].params.Get("domain"))
}

func TestReadGone(t *testing.T) {
	for name, respond := range map[string]func(string, url.Values) (map[string]any, error){
		"no match": func(string, url.Values) (map[string]any, error) {
			return map[string]any{"records": []any{record("10.0.0.1", 60)}}, nil
		},
		"not found": func(string, url.Values) (map[string]any, error) {
			return nil, &client.Error{Message: "No such zone was found: example.com"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			state, err := (&Resource{spec: recordA, client: &mockAPI{respond: respond}}).read(context.Background(), identity())
			require.NoError(t, err)
			assert.Nil(t, state)
		})
	}
}

func TestReadError(t *testing.T) {
	api := &mockAPI{respond: func(string, url.Values) (map[string]any, error) {
		return nil, &client.Error{Message: "Access was denied."}
	}}
	_, err := (&Resource{spec: recordA, client: api}).read(context.Background(), identity())
	assert.Error(t, err)
}

// The server resets ttl, disable and comments when an update leaves them out,
// so an update sends every known value, not only the changed one.
func TestUpdateSendsAllValues(t *testing.T) {
	api := &mockAPI{}
	r := &Resource{spec: recordA, client: api}

	prior := identity()
	prior["ttl"], prior["comments"], prior["disable"] = num(300), str("old"), boolean(true)
	plan := identity()
	plan["ttl"], plan["comments"], plan["disable"] = unknown(Int64), str("new"), boolean(true)

	require.NoError(t, r.update(context.Background(), prior, plan))
	require.Len(t, api.calls, 1)
	p := api.calls[0].params
	assert.Equal(t, url.Values{
		"type": {"A"}, "zone": {"example.com"}, "domain": {"www.example.com"}, "ipAddress": {"10.0.0.2"},
		"ttl": {"300"}, "comments": {"new"}, "disable": {"true"},
	}, p)
}

func TestUpdateWithoutChanges(t *testing.T) {
	api := &mockAPI{}
	prior := identity()
	prior["ttl"] = num(300)
	require.NoError(t, (&Resource{spec: recordA, client: api}).update(context.Background(), prior, prior))
	assert.Empty(t, api.calls)
}

func TestCreateAppliesUpdateOnlyValues(t *testing.T) {
	api := &mockAPI{}
	r := &Resource{spec: recordA, client: api}

	plan := identity()
	plan["ptr"], plan["disable"] = boolean(true), boolean(true)
	plan["ttl"], plan["comments"] = unknown(Int64), unknown(String)

	require.NoError(t, r.create(context.Background(), plan))
	add := api.calls[0].params
	assert.Equal(t, "true", add.Get("ptr"))
	assert.False(t, add.Has("disable"), "add does not take disable")
	assert.False(t, add.Has("ttl"), "unknown values are not sent")
	assert.True(t, r.hasUpdateOnly(plan))
}

func TestCreateParamNames(t *testing.T) {
	api := &mockAPI{}
	spec := Spec{Name: "zone", Create: "create", Read: "get", Update: "set", Delete: "delete",
		CreateFixed: map[string]string{"initializeForwarder": "false"},
		Attrs: []Attr{
			{Name: "zone", Param: "zone", Type: String, Key: "name", Required: true, Identity: true, Create: true, Update: true},
			{Name: "protocol", Param: "primaryZoneTransferProtocol", CreateParam: "zoneTransferProtocol", Type: String, Key: "primaryZoneTransferProtocol", Create: true, Update: true},
		}}
	r := &Resource{spec: spec, client: api}
	plan := map[string]tftypes.Value{"zone": str("example.com"), "protocol": str("Tls")}

	require.NoError(t, r.create(context.Background(), plan))
	assert.Equal(t, url.Values{"zone": {"example.com"}, "zoneTransferProtocol": {"Tls"}, "initializeForwarder": {"false"}}, api.calls[0].params)

	prior := map[string]tftypes.Value{"zone": str("example.com"), "protocol": str("Tcp")}
	require.NoError(t, r.update(context.Background(), prior, plan))
	assert.Equal(t, url.Values{"zone": {"example.com"}, "primaryZoneTransferProtocol": {"Tls"}}, api.calls[1].params)
}

func TestCreateAdopts(t *testing.T) {
	spec := Spec{Name: "record_soa", Read: "get", Update: "update", List: "records", Fixed: map[string]string{"type": "SOA"}, Adopt: true,
		Attrs: []Attr{
			{Name: "zone", Param: "zone", Type: String, Required: true, Identity: true, Update: true},
			{Name: "serial", Param: "serial", Type: Int64, Key: "rData.serial", ReadOnly: true, Update: true},
			{Name: "refresh", Param: "refresh", Type: Int64, Key: "rData.refresh", Update: true},
			{Name: "retry", Param: "retry", Type: Int64, Key: "rData.retry", Update: true},
		}}
	api := &mockAPI{respond: func(path string, _ url.Values) (map[string]any, error) {
		return map[string]any{"records": []any{map[string]any{"type": "SOA", "rData": map[string]any{
			"serial": json.Number("7"), "refresh": json.Number("900"), "retry": json.Number("300"),
		}}}}, nil
	}}
	plan := map[string]tftypes.Value{"zone": str("example.com"), "serial": unknown(Int64), "refresh": num(7200), "retry": unknown(Int64)}

	require.NoError(t, (&Resource{spec: spec, client: api}).create(context.Background(), plan))
	assert.Equal(t, []string{"get", "update"}, api.paths())
	assert.Equal(t, url.Values{"zone": {"example.com"}, "type": {"SOA"}, "serial": {"7"}, "refresh": {"7200"}, "retry": {"300"}}, api.calls[1].params,
		"the update gets every value, the unconfigured ones from the server")
}

func TestSingletonCreateUsesUpdate(t *testing.T) {
	api := &mockAPI{}
	spec := Spec{Name: "settings", Read: "get", Update: "set", Attrs: []Attr{
		{Name: "forwarders", Param: "forwarders", Type: List, Key: "forwarders", Update: true},
	}}
	plan := map[string]tftypes.Value{"forwarders": tftypes.NewValue(List.tf(), []tftypes.Value{})}

	require.NoError(t, (&Resource{spec: spec, client: api}).create(context.Background(), plan))
	assert.Equal(t, []string{"set"}, api.paths())
	assert.Equal(t, url.Values{"forwarders": {""}}, api.calls[0].params, "an empty list clears the setting")
	assert.True(t, spec.Singleton())
}

func TestReadKeepsEmptyList(t *testing.T) {
	spec := Spec{Name: "settings", Read: "get", Update: "set", Attrs: []Attr{
		{Name: "forwarders", Param: "forwarders", Type: List, Key: "forwarders", Update: true},
	}}
	empty := tftypes.NewValue(List.tf(), []tftypes.Value{})
	api := &mockAPI{respond: func(string, url.Values) (map[string]any, error) { return map[string]any{"forwarders": nil}, nil }}

	state, err := (&Resource{spec: spec, client: api}).read(context.Background(), map[string]tftypes.Value{"forwarders": empty})
	require.NoError(t, err)
	assert.True(t, empty.Equal(state["forwarders"]))

	state, err = (&Resource{spec: spec, client: api}).read(context.Background(), map[string]tftypes.Value{"forwarders": tftypes.NewValue(List.tf(), nil)})
	require.NoError(t, err)
	assert.True(t, state["forwarders"].IsNull())
}

// The server returns names without the trailing dot and may change their case;
// the configured spelling must survive a read or the apply is inconsistent.
func TestReadKeepsEquivalentNames(t *testing.T) {
	api := &mockAPI{respond: func(string, url.Values) (map[string]any, error) {
		rec := record("10.0.0.2", 300)
		rec["comments"] = "hello"
		return map[string]any{"records": []any{rec}}, nil
	}}
	prior := identity()
	prior["domain"] = str("WWW.example.com.")
	prior["comments"] = str("Hello")
	state, err := (&Resource{spec: recordA, client: api}).read(context.Background(), prior)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, str("WWW.example.com."), state["domain"])
	assert.Equal(t, str("hello"), state["comments"], "case-only drift outside identity attributes must show")
}

func schemaOf(r *Resource) schema.Schema {
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	return resp.Schema
}

func nullState(s schema.Schema) tfsdk.State {
	return tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}
}

// object builds a full resource value, null where vals has nothing.
func object(s schema.Schema, vals map[string]tftypes.Value) tftypes.Value {
	typ := s.Type().TerraformType(context.Background()).(tftypes.Object)
	out := map[string]tftypes.Value{}
	for name, t := range typ.AttributeTypes {
		out[name] = tftypes.NewValue(t, nil)
		if v, ok := vals[name]; ok {
			out[name] = v
		}
	}
	return tftypes.NewValue(typ, out)
}

// Once the object exists, a later failure still records it so Terraform
// taints it instead of losing track of it.
func TestCreateKeepsStateOnLaterFailure(t *testing.T) {
	api := &mockAPI{respond: func(path string, _ url.Values) (map[string]any, error) {
		switch path {
		case "get":
			return map[string]any{"records": []any{record("10.0.0.2", 300)}}, nil
		case "update":
			return nil, errors.New("boom")
		}
		return map[string]any{}, nil
	}}
	r := &Resource{spec: recordA, client: api}
	s := schemaOf(r)
	plan := identity()
	plan["disable"] = boolean(true)
	plan["id"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	plan["ttl"], plan["comments"] = unknown(Int64), unknown(String)

	resp := &resource.CreateResponse{State: nullState(s)}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: object(s, plan)}}, resp)

	assert.True(t, resp.Diagnostics.HasError())
	require.False(t, resp.State.Raw.IsNull())
	var id types.String
	resp.State.GetAttribute(context.Background(), path.Root("id"), &id)
	assert.Equal(t, "example.com/www.example.com/10.0.0.2", id.ValueString())
}

// Optional identity values may be left empty in an import id.
func TestImportEmptyPartIsNull(t *testing.T) {
	txt := Spec{Name: "record_txt", Create: "add", Read: "get", Update: "update", Delete: "delete", List: "records", Attrs: []Attr{
		{Name: "zone", Param: "zone", Type: String, Required: true, Identity: true, Create: true},
		{Name: "domain", Param: "domain", Type: String, Key: "name", Required: true, Identity: true, Create: true},
		{Name: "base64", Param: "b64", Type: List, Key: "rData.b64", Identity: true, Create: true},
		{Name: "text", Param: "text", Type: String, Key: "rData.text", Identity: true, Create: true},
		{Name: "split", Param: "split", Type: Bool, Key: "rData.split", Identity: true, Create: true},
	}}
	r := &Resource{spec: txt}
	s := schemaOf(r)
	resp := &resource.ImportStateResponse{State: nullState(s)}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "example.com/txt.example.com//hello/"}, resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)

	var split types.Bool
	var b64 types.List
	var text types.String
	resp.State.GetAttribute(context.Background(), path.Root("split"), &split)
	resp.State.GetAttribute(context.Background(), path.Root("base64"), &b64)
	resp.State.GetAttribute(context.Background(), path.Root("text"), &text)
	assert.True(t, split.IsNull())
	assert.True(t, b64.IsNull())
	assert.Equal(t, "hello", text.ValueString())
}

func TestImportJSON(t *testing.T) {
	txt := Spec{Name: "record_txt", Create: "add", Read: "get", Update: "update", Delete: "delete", List: "records", Attrs: []Attr{
		{Name: "zone", Param: "zone", Type: String, Required: true, Identity: true, Create: true},
		{Name: "domain", Param: "domain", Type: String, Key: "name", Required: true, Identity: true, Create: true},
		{Name: "base64", Param: "b64", Type: List, Key: "rData.b64", Identity: true, Create: true},
		{Name: "text", Param: "text", Type: String, Key: "rData.text", Identity: true, Create: true},
		{Name: "split", Param: "split", Type: Bool, Key: "rData.split", Identity: true, Create: true},
		{Name: "ttl", Param: "ttl", Type: Int64, Key: "ttl", Create: true},
	}}
	r := &Resource{spec: txt}
	s := schemaOf(r)

	resp := &resource.ImportStateResponse{State: nullState(s)}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: `{"zone": "example.com", "domain": "txt.example.com", "text": "v=spf1 ip4:10.0.0.0/8 -all", "split": true, "base64": ["YS9i"]}`}, resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
	var text types.String
	var split types.Bool
	var b64 []string
	resp.State.GetAttribute(context.Background(), path.Root("text"), &text)
	resp.State.GetAttribute(context.Background(), path.Root("split"), &split)
	resp.State.GetAttribute(context.Background(), path.Root("base64"), &b64)
	assert.Equal(t, "v=spf1 ip4:10.0.0.0/8 -all", text.ValueString())
	assert.True(t, split.ValueBool())
	assert.Equal(t, []string{"YS9i"}, b64)

	for _, id := range []string{`{"zone": "example.com", "ttl": 60}`, `{"zone": `} {
		resp = &resource.ImportStateResponse{State: nullState(s)}
		r.ImportState(context.Background(), resource.ImportStateRequest{ID: id}, resp)
		assert.True(t, resp.Diagnostics.HasError(), id)
	}
}

func TestReadOnlyNeverReplaces(t *testing.T) {
	assert.False(t, Attr{Name: "is_sso_user", Type: Bool, Key: "isSsoUser", ReadOnly: true}.ForceNew())
	assert.True(t, Attr{Name: "ptr", Type: Bool, Create: true}.ForceNew(), "a value only create takes still replaces")
}

func TestToggleAndSource(t *testing.T) {
	spec := Spec{Name: "dhcp_scope", Read: "get", Update: "set", Delete: "delete", Attrs: []Attr{
		{Name: "name", Param: "name", Type: String, Key: "name", Required: true, Identity: true, Update: true},
		{Name: "dns_ttl", Param: "dnsTtl", Type: Int64, Key: "dnsTtl", Update: true},
		{Name: "enabled", Param: "enabled", Type: Bool, Key: "enabled", Update: true,
			Toggle: &Toggle{On: "enable", Off: "disable"}, Source: &Source{Path: "list", List: "scopes"}},
	}}
	api := &mockAPI{respond: func(path string, _ url.Values) (map[string]any, error) {
		if path == "list" {
			return map[string]any{"scopes": []any{
				map[string]any{"name": "other", "enabled": true},
				map[string]any{"name": "lan", "enabled": false},
			}}, nil
		}
		return map[string]any{"name": "lan", "dnsTtl": json.Number("900")}, nil
	}}
	r := &Resource{spec: spec, client: api}

	plan := map[string]tftypes.Value{"name": str("lan"), "dns_ttl": unknown(Int64), "enabled": boolean(false)}
	require.NoError(t, r.create(context.Background(), plan))
	assert.Equal(t, []string{"set", "disable"}, api.paths())
	assert.False(t, api.calls[0].params.Has("enabled"), "a toggle is not a parameter")
	assert.Equal(t, url.Values{"name": {"lan"}}, api.calls[1].params)

	state, err := r.read(context.Background(), plan)
	require.NoError(t, err)
	assert.True(t, boolean(false).Equal(state["enabled"]), "read from the matching list item")
	assert.True(t, num(900).Equal(state["dns_ttl"]))

	api.calls = nil
	prior := map[string]tftypes.Value{"name": str("lan"), "dns_ttl": num(900), "enabled": boolean(false)}
	next := map[string]tftypes.Value{"name": str("lan"), "dns_ttl": num(900), "enabled": boolean(true)}
	require.NoError(t, r.update(context.Background(), prior, next))
	assert.Equal(t, []string{"enable"}, api.paths(), "only the toggle changed")
}

func TestCreatedValues(t *testing.T) {
	spec := Spec{Name: "api_token", Create: "create", Read: "list", Delete: "delete", List: "sessions",
		Created: func(resp map[string]any) map[string]any {
			token, _ := resp["token"].(string)
			return map[string]any{"token": token, "partial_token": token[:4]}
		},
		Attrs: []Attr{
			{Name: "token_name", Param: "tokenName", Type: String, Key: "tokenName", Required: true, Create: true},
			{Name: "partial_token", Param: "partialToken", Type: String, Key: "partialToken", Identity: true, ReadOnly: true},
			{Name: "token", Param: "token", Type: String, Sensitive: true, ReadOnly: true},
		}}
	api := &mockAPI{respond: func(path string, _ url.Values) (map[string]any, error) {
		if path == "create" {
			return map[string]any{"token": "abcd1234"}, nil
		}
		return map[string]any{"sessions": []any{
			map[string]any{"partialToken": "ffff", "tokenName": "ci"},
			map[string]any{"partialToken": "abcd", "tokenName": "ci"},
		}}, nil
	}}
	r := &Resource{spec: spec, client: api}
	plan := map[string]tftypes.Value{"token_name": str("ci"), "partial_token": unknown(String), "token": unknown(String)}

	require.NoError(t, r.create(context.Background(), plan))
	assert.False(t, api.calls[0].params.Has("partialToken"), "not known before create")
	state, err := r.read(context.Background(), plan)
	require.NoError(t, err)
	assert.True(t, str("abcd").Equal(state["partial_token"]), "found by the created identity")
	assert.True(t, str("abcd1234").Equal(state["token"]), "the token is kept from create")
	assert.True(t, str("abcd").Equal(state["id"]))
}

func TestCreateCutOff(t *testing.T) {
	exists := false
	api := &mockAPI{respond: func(path string, _ url.Values) (map[string]any, error) {
		switch path {
		case "add":
			exists = true // the server got it, but the answer was lost
			return nil, fmt.Errorf("post: %w", io.EOF)
		case "get":
			if !exists {
				return map[string]any{"records": []any{}}, nil
			}
			return map[string]any{"records": []any{record("10.0.0.2", 60)}}, nil
		}
		return map[string]any{}, nil
	}}
	r := &Resource{spec: recordA, client: api}
	require.NoError(t, r.create(context.Background(), identity()), "the record exists, so the create happened")

	exists = false
	api.respond = func(path string, _ url.Values) (map[string]any, error) {
		if path == "add" {
			return nil, fmt.Errorf("post: %w", io.EOF)
		}
		return map[string]any{"records": []any{}}, nil
	}
	assert.Error(t, r.create(context.Background(), identity()), "nothing was created")
}
