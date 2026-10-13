package framework

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromJSON(t *testing.T) {
	list := func(s ...string) tftypes.Value {
		var v []tftypes.Value
		for _, e := range s {
			v = append(v, str(e))
		}
		return tftypes.NewValue(List.tf(), v)
	}
	for _, tc := range []struct {
		raw  any
		typ  Type
		want tftypes.Value
	}{
		{"x", String, str("x")},
		{json.Number("5"), String, str("5")},
		{json.Number("300"), Int64, num(300)},
		{"300", Int64, num(300)},
		{true, Bool, boolean(true)},
		{"false", Bool, boolean(false)},
		{[]any{"1.1.1.1", "9.9.9.9"}, List, list("1.1.1.1", "9.9.9.9")},
		{"a, b", List, list("a", "b")},
		{"", List, list()},
		{nil, String, tftypes.NewValue(tftypes.String, nil)},
	} {
		got, err := fromJSON(tc.raw, tc.typ)
		require.NoError(t, err, "%v", tc.raw)
		assert.True(t, tc.want.Equal(got), "%v as %s: got %v", tc.raw, tc.typ, got)
	}

	for _, tc := range []struct {
		raw any
		typ Type
	}{
		{map[string]any{"alpn": "h2"}, String},
		{[]any{map[string]any{}}, List},
		{"x", Int64},
	} {
		_, err := fromJSON(tc.raw, tc.typ)
		assert.Error(t, err, "%v as %s", tc.raw, tc.typ)
	}
}

func TestParamRoundTrip(t *testing.T) {
	for _, typ := range []Type{String, Int64, Bool, List} {
		for _, s := range map[Type][]string{String: {"a"}, Int64: {"42"}, Bool: {"true"}, List: {"a,b", ""}}[typ] {
			v, err := fromJSON(s, typ)
			require.NoError(t, err)
			got, err := param(v, typ)
			require.NoError(t, err)
			assert.Equal(t, s, got)
		}
	}
}

func TestLookup(t *testing.T) {
	obj := map[string]any{"proxy": map[string]any{"type": "Http"}, "rData": nil}
	v, ok := lookup(obj, "proxy.type")
	assert.True(t, ok)
	assert.Equal(t, "Http", v)
	_, ok = lookup(obj, "rData.ipAddress")
	assert.False(t, ok)
}

func TestTable(t *testing.T) {
	a := Attr{Name: "limits", Param: "limits", Type: Table, Columns: []Column{
		{Name: "prefix", Key: "prefix", Type: Int64},
		{Name: "udp_limit", Key: "udpLimit", Type: Int64},
		{Name: "label", Key: "label", Type: String},
	}}
	row := func(p, l int64, s string) tftypes.Value {
		return tftypes.NewValue(a.row(), map[string]tftypes.Value{"prefix": num(p), "udp_limit": num(l), "label": str(s)})
	}
	rows := tftypes.NewValue(a.tf(), []tftypes.Value{row(32, 600, "a"), row(24, 6000, "b")})

	s, err := a.param(rows)
	require.NoError(t, err)
	assert.Equal(t, "32|600|a|24|6000|b", s)

	s, err = a.param(tftypes.NewValue(a.tf(), []tftypes.Value{}))
	require.NoError(t, err)
	assert.Empty(t, s, "no rows clears the table")

	got, err := a.fromJSON([]any{
		map[string]any{"prefix": json.Number("32"), "udpLimit": json.Number("600"), "label": "a", "extra": true},
		map[string]any{"prefix": json.Number("24"), "udpLimit": json.Number("6000"), "label": "b"},
	})
	require.NoError(t, err)
	assert.True(t, rows.Equal(got), "got %s", got)

	got, err = a.fromJSON(nil)
	require.NoError(t, err)
	assert.True(t, got.IsNull())
	assert.True(t, equivalent(a, got, tftypes.NewValue(a.tf(), []tftypes.Value{})), "null keeps an empty table")

	_, err = a.fromJSON([]any{"x"})
	assert.Error(t, err)
}

func TestMap(t *testing.T) {
	m := tftypes.NewValue(Map.tf(), map[string]tftypes.Value{"port": str("8443"), "alpn": str("h2,h3")})

	s, err := param(m, Map)
	require.NoError(t, err)
	assert.Equal(t, "alpn|h2,h3|port|8443", s, "keys are sorted")

	got, err := fromJSON(map[string]any{"alpn": "h2,h3", "port": "8443"}, Map)
	require.NoError(t, err)
	assert.True(t, m.Equal(got))

	got, err = fromJSON("alpn|h2,h3|port|8443", Map)
	require.NoError(t, err)
	assert.True(t, m.Equal(got), "import IDs use the parameter form")

	_, err = fromJSON("alpn|h2|port", Map)
	assert.Error(t, err)
}

func TestTableListColumn(t *testing.T) {
	a := Attr{Name: "policies", Param: "policies", Type: Table, Columns: []Column{
		{Name: "key", Key: "tsigKeyName", Type: String},
		{Name: "types", Key: "allowedTypes", Type: List},
	}}
	types := tftypes.NewValue(List.tf(), []tftypes.Value{str("A"), str("AAAA")})
	rows := tftypes.NewValue(a.tf(), []tftypes.Value{tftypes.NewValue(a.row(), map[string]tftypes.Value{"key": str("k"), "types": types})})

	s, err := a.param(rows)
	require.NoError(t, err)
	assert.Equal(t, "k|A,AAAA", s)

	got, err := a.fromJSON([]any{map[string]any{"tsigKeyName": "k", "allowedTypes": []any{"A", "AAAA"}}})
	require.NoError(t, err)
	assert.True(t, rows.Equal(got), "got %s", got)
}
