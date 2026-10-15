package terraforming

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"

	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
	"github.com/ilijamt/terraform-provider-technitium/internal/technitium"
)

// attr evaluates an attribute of the first block without variables or
// functions, as Terraform 1.5 does for an import ID.
func attr(t *testing.T, f *hclwrite.File, name string) cty.Value {
	t.Helper()
	file, diags := hclsyntax.ParseConfig(f.Bytes(), "out.tf", hcl.InitialPos)
	require.False(t, diags.HasErrors(), "%s\n%s", diags.Error(), f.Bytes())
	v, diags := file.Body.(*hclsyntax.Body).Blocks[0].Body.Attributes[name].Expr.Value(nil)
	require.False(t, diags.HasErrors(), "%s\n%s", diags.Error(), f.Bytes())
	return v
}

func TestImportIDIsLiteral(t *testing.T) {
	for _, id := range []string{
		`export.test/www.export.test/10.0.0.1`,
		`{"zone":"a.test","domain":"a.test","text":"a/b ${x} %{y}"}`,
	} {
		f := hclwrite.NewEmptyFile()
		importBlock(f.Body(), "record_txt", "a_test", id)
		assert.Equal(t, cty.StringVal(id), attr(t, f, "id"), "%s", f.Bytes())
	}
}

func TestKeywordKeys(t *testing.T) {
	keys := []string{"for", "in", "if", "null", "true", "false", "else", "endfor", "endif", "plain"}
	for _, k := range keys {
		f := hclwrite.NewEmptyFile()
		obj := tftypes.NewValue(tftypes.Map{ElementType: tftypes.Number}, map[string]tftypes.Value{k: tftypes.NewValue(tftypes.Number, 1)})
		f.Body().AppendNewBlock("b", nil).Body().SetAttributeRaw("v", writer{}.tokens(obj, []string{"v"}))
		assert.True(t, cty.ObjectVal(map[string]cty.Value{k: cty.NumberIntVal(1)}).Equals(attr(t, f, "v")).True(), "%s", f.Bytes())
	}

	for _, k := range keys {
		testJSONKeys(t, `{"`+k+`":1}`)
	}
	testJSONKeys(t, `{"for":0,"in":1,"if":2,"null":3,"true":4,"false":5,"else":6,"endfor":7,"endif":8,"plain":9}`)
}

func TestForceRemovesStaleFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"records_gone_test.tf", "dhcp.tf", "imports.tf"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(marker+"# old\n"), 0o644))
	}
	for _, name := range []string{"variables.tf", "main.tf", "records.txt", "backend.tf", "records_mine.tf"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("# mine\n"), 0o644))
	}
	out := map[string]*hclwrite.File{"zones.tf": hclwrite.NewEmptyFile(), "imports.tf": hclwrite.NewEmptyFile(), "records_kept_test.tf": hclwrite.NewEmptyFile()}
	require.NoError(t, write(dir, out, true, true, io.Discard))

	assert.ElementsMatch(t, []string{"zones.tf", "imports.tf", "records_kept_test.tf", "variables.tf", "main.tf", "records.txt", "backend.tf", "records_mine.tf"}, names(t, dir),
		"only marked files are removed")
	data, err := os.ReadFile(filepath.Join(dir, "zones.tf"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), marker), "%s", data)
}

func TestFilteredForceKeepsFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"zones.tf", "records_a_test.tf", "imports.tf"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(marker), 0o644))
	}
	out := map[string]*hclwrite.File{"apps.tf": hclwrite.NewEmptyFile(), "imports.tf": hclwrite.NewEmptyFile()}
	require.NoError(t, write(dir, out, true, false, io.Discard))
	assert.ElementsMatch(t, []string{"zones.tf", "records_a_test.tf", "imports.tf", "apps.tf"}, names(t, dir))

	assert.True(t, Options{Force: true}.prune(false))
	assert.False(t, Options{}.prune(false))
	assert.False(t, Options{Force: true, Types: []string{"app"}}.prune(false))
	assert.False(t, Options{Force: true, Zones: []string{"a.test"}}.prune(false))
	assert.False(t, Options{Force: true}.prune(true), "an object that failed to read keeps its earlier file")
}

func TestReadAllFailed(t *testing.T) {
	var warnings []string
	warn := func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }
	objs := []object{{Type: "zone", ID: "a.test", name: "a"}, {Type: "zone", ID: "gone.test", name: "gone"}, {Type: "user", ID: "x", name: "x", Data: true}}
	read := func(_ context.Context, _, id string) (map[string]tftypes.Value, error) {
		switch id {
		case "gone.test":
			return nil, nil
		case "broken.test":
			return nil, errors.New("boom")
		}
		return map[string]tftypes.Value{}, nil
	}

	kept, states, failed := readAll(context.Background(), objs, read, warn)
	assert.Len(t, kept, 2)
	assert.Contains(t, states, "zone.a")
	assert.False(t, failed, "a gone object is not a failure")

	_, _, failed = readAll(context.Background(), append(objs, object{Type: "zone", ID: "broken.test", name: "broken"}), read, warn)
	assert.True(t, failed)
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestDNSSECRecordsSkippedQuietly(t *testing.T) {
	specs := map[string]framework.Spec{}
	for _, s := range technitium.Specs() {
		specs[s.Name] = s
	}
	var warnings []string
	rec := func(typ string, rdata map[string]any) any {
		return map[string]any{"name": "s.test", "type": typ, "rData": rdata}
	}
	l := &lister{api: fakeAPI{
		"/api/zones/list": {"zones": []any{map[string]any{"name": "s.test", "type": "Primary", "dnssecStatus": "SignedWithNSEC3"}}},
		"/api/zones/records/get": {"records": []any{
			rec("SOA", nil), rec("DNSKEY", nil), rec("RRSIG", nil), rec("NSEC", nil), rec("NSEC3", nil), rec("NSEC3PARAM", nil),
			rec("DS", map[string]any{"keyTag": json.Number("1"), "algorithm": "ECDSAP256SHA256", "digestType": "SHA256", "digest": "AB"}),
			rec("HINFO", nil),
		}},
	}, specs: specs, warn: func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }}

	objs, err := l.listZones(context.Background())
	require.NoError(t, err)
	var types []string
	for _, o := range objs {
		types = append(types, o.Type)
	}
	assert.Contains(t, types, "record_ds", "DS is still exported")
	assert.Len(t, warnings, 1, "%q", warnings)
	assert.Contains(t, warnings[0], "HINFO")
}

func TestAppConfigSensitive(t *testing.T) {
	var attr framework.Attr
	for _, s := range technitium.Specs() {
		if s.Name == "app_config" {
			for _, a := range s.Attrs {
				if a.Name == "config" {
					attr = a
				}
			}
		}
	}
	require.True(t, attr.Sensitive, "app_config.config is sensitive")

	schema := &tfprotov6.Schema{Block: &tfprotov6.SchemaBlock{Attributes: []*tfprotov6.SchemaAttribute{
		{Name: "name", Type: tftypes.String, Required: true},
		{Name: "config", Type: tftypes.String, Required: true, Sensitive: true},
	}}}
	f := hclwrite.NewEmptyFile()
	vars := resourceBlock(f.Body(), "app_config", "x", schema, map[string]tftypes.Value{
		"name":   tftypes.NewValue(tftypes.String, "X"),
		"config": tftypes.NewValue(tftypes.String, `{"lan":["10.0.0.0/8"]}`),
	}, nil, func(a string) bool { return a == "config" })
	assert.Empty(t, vars, "the config stays a literal, so the import plans no change")
	assert.Contains(t, string(f.Bytes()), "jsonencode")

	assert.Equal(t, []string{"apiKey", "password", "token"}, secretKeys(`{"enable":true,"Auth":{"password":"x","apiKey":"y"},"list":[{"token":"z"}],"networks":{"lan":[]}}`))
	assert.Empty(t, secretKeys(`{"networks":{"lan":["10.0.0.0/8"]}}`))
	assert.Empty(t, secretKeys(`not json`))
}

func testJSONKeys(t *testing.T, js string) {
	t.Helper()
	tokens, ok := jsonencode(js)
	require.True(t, ok)
	f := hclwrite.NewEmptyFile()
	f.Body().AppendNewBlock("b", nil).Body().SetAttributeRaw("v", tokens)
	file, diags := hclsyntax.ParseConfig(f.Bytes(), "out.tf", hcl.InitialPos)
	require.False(t, diags.HasErrors(), "%s\n%s", diags.Error(), f.Bytes())
	v, diags := file.Body.(*hclsyntax.Body).Blocks[0].Body.Attributes["v"].Expr.Value(&hcl.EvalContext{
		Functions: map[string]function.Function{"jsonencode": stdlib.JSONEncodeFunc},
	})
	require.False(t, diags.HasErrors(), "%s\n%s", diags.Error(), f.Bytes())
	assert.JSONEq(t, js, v.AsString(), "%s", f.Bytes())
}
