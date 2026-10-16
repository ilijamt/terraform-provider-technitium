package framework

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var restartingSettings = Spec{Name: "settings", Read: "get", Update: "set", Attrs: []Attr{
	{Name: "port", Param: "port", Type: Int64, Key: "port", Update: true, Restarts: true},
	{Name: "secret", Param: "secret", Type: String, Update: true, Restarts: true},
	{Name: "header", Param: "header", Type: String, Key: "header", Update: true},
	{Name: "on", Param: "on", Type: Bool, Key: "on", Update: true, Toggle: &Toggle{On: "enable", Off: "disable"}},
}}

func (m *mockAPI) marks() []bool {
	var out []bool
	for _, c := range m.calls {
		out = append(out, c.restarts)
	}
	return out
}

func settingsValues(port int64, secret, header string, on bool) map[string]tftypes.Value {
	return map[string]tftypes.Value{"port": num(port), "secret": str(secret), "header": str(header), "on": boolean(on)}
}

func TestUpdateMarksRestarts(t *testing.T) {
	prior := settingsValues(5380, "s", "X-Real-IP", false)
	for name, tc := range map[string]struct {
		plan  map[string]tftypes.Value
		paths []string
		marks []bool
	}{
		"restarting value changed":     {settingsValues(5381, "s", "X-Real-IP", false), []string{"set"}, []bool{true}},
		"value without a key changed":  {settingsValues(5380, "t", "X-Real-IP", false), []string{"set"}, []bool{true}},
		"only another value changed":   {settingsValues(5380, "s", "X-Forwarded-For", false), []string{"set"}, []bool{false}},
		"unknown is the prior value":   {map[string]tftypes.Value{"port": unknown(Int64), "secret": str("s"), "header": str("X-Forwarded-For"), "on": boolean(false)}, []string{"set"}, []bool{false}},
		"mark stays on the set call":   {settingsValues(5381, "s", "X-Real-IP", true), []string{"set", "enable"}, []bool{true, false}},
		"toggle alone is not a change": {settingsValues(5380, "s", "X-Real-IP", true), []string{"enable"}, []bool{false}},
	} {
		t.Run(name, func(t *testing.T) {
			api := &mockAPI{}
			require.NoError(t, (&Resource{spec: restartingSettings, client: api}).update(context.Background(), prior, tc.plan))
			assert.Equal(t, tc.paths, api.paths())
			assert.Equal(t, tc.marks, api.marks())
		})
	}
}

// A singleton is created with its update call, so only a value the server
// does not have yet restarts it.
func TestSingletonCreateMarksRestarts(t *testing.T) {
	unset := func(port tftypes.Value, header string) map[string]tftypes.Value {
		return map[string]tftypes.Value{"port": port, "secret": tftypes.NewValue(tftypes.String, nil), "header": str(header), "on": unknown(Bool)}
	}
	for name, tc := range map[string]struct {
		plan  map[string]tftypes.Value
		paths []string
		marks []bool
	}{
		"server has the value":         {unset(num(5380), "X-Real-IP"), []string{"get", "set"}, []bool{false, false}},
		"server has another value":     {unset(num(5381), "X-Real-IP"), []string{"get", "set"}, []bool{false, true}},
		"no restarting value":          {unset(unknown(Int64), "X-Forwarded-For"), []string{"set"}, []bool{false}},
		"value the server never shows": {map[string]tftypes.Value{"port": unknown(Int64), "secret": str("s"), "header": str("X-Real-IP"), "on": unknown(Bool)}, []string{"set"}, []bool{true}},
	} {
		t.Run(name, func(t *testing.T) {
			api := &mockAPI{respond: func(path string, _ url.Values) (map[string]any, error) {
				if path == "get" {
					return map[string]any{"port": json.Number("5380"), "header": "X-Real-IP", "on": false}, nil
				}
				return map[string]any{}, nil
			}}
			require.NoError(t, (&Resource{spec: restartingSettings, client: api}).create(context.Background(), tc.plan))
			assert.Equal(t, tc.paths, api.paths())
			assert.Equal(t, tc.marks, api.marks())
		})
	}
}
