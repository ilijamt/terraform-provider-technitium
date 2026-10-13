package provider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tp "github.com/ilijamt/terraform-provider-technitium/internal/provider"
)

func configure(t *testing.T, cfg map[string]tftypes.Value) *provider.ConfigureResponse {
	t.Helper()
	ctx := context.Background()
	p := tp.New("test", nil, nil)()
	var sresp provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &sresp)
	typ := sresp.Schema.Type().TerraformType(ctx).(tftypes.Object)

	vals := map[string]tftypes.Value{}
	for name, at := range typ.AttributeTypes {
		vals[name] = tftypes.NewValue(at, nil)
	}
	for name, v := range cfg {
		vals[name] = v
	}
	var resp provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{Config: tfsdk.Config{Schema: sresp.Schema, Raw: tftypes.NewValue(typ, vals)}}, &resp)
	return &resp
}

func str(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }

func clearEnv(t *testing.T) {
	for _, k := range []string{"TECHNITIUM_HOST", "TECHNITIUM_TOKEN", "TECHNITIUM_USERNAME", "TECHNITIUM_PASSWORD", "TECHNITIUM_INSECURE"} {
		t.Setenv(k, "")
	}
}

func loginServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.URL.Path == "/api/user/login" && r.Form.Get("user") == "admin" && r.Form.Get("pass") == "secret" {
			_, _ = w.Write([]byte(`{"status":"ok","token":"t0k"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"error","errorMessage":"Invalid username or password"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestConfigureUnknown(t *testing.T) {
	clearEnv(t)
	srv := loginServer(t)
	t.Setenv("TECHNITIUM_HOST", srv.URL)
	t.Setenv("TECHNITIUM_USERNAME", "admin")
	t.Setenv("TECHNITIUM_PASSWORD", "secret")

	resp := configure(t, map[string]tftypes.Value{"token": tftypes.NewValue(tftypes.String, tftypes.UnknownValue)})
	require.True(t, resp.Diagnostics.HasError(), "unknown token must not fall back to the environment")
	assert.Nil(t, resp.ResourceData)
}

func TestConfigureCredentials(t *testing.T) {
	srv := loginServer(t)
	for name, tc := range map[string]struct {
		env     map[string]string
		cfg     map[string]tftypes.Value
		wantErr string
	}{
		"config login beats env token": {
			env: map[string]string{"TECHNITIUM_TOKEN": "env"},
			cfg: map[string]tftypes.Value{"username": str("admin"), "password": str("secret")},
		},
		"config username with env password": {
			env: map[string]string{"TECHNITIUM_PASSWORD": "secret"},
			cfg: map[string]tftypes.Value{"username": str("admin")},
		},
		"config token beats env login": {
			env: map[string]string{"TECHNITIUM_USERNAME": "admin", "TECHNITIUM_PASSWORD": "wrong"},
			cfg: map[string]tftypes.Value{"token": str("t0k")},
		},
		"env login": {
			env: map[string]string{"TECHNITIUM_USERNAME": "admin", "TECHNITIUM_PASSWORD": "secret"},
		},
		"both in config": {
			cfg:     map[string]tftypes.Value{"token": str("t0k"), "username": str("admin"), "password": str("secret")},
			wantErr: "Conflicting credentials",
		},
		"both in env": {
			env:     map[string]string{"TECHNITIUM_TOKEN": "t0k", "TECHNITIUM_USERNAME": "admin", "TECHNITIUM_PASSWORD": "secret"},
			wantErr: "Conflicting credentials",
		},
		"missing password": {
			cfg:     map[string]tftypes.Value{"username": str("admin")},
			wantErr: "Missing credentials",
		},
		"bad insecure env": {
			env:     map[string]string{"TECHNITIUM_TOKEN": "t0k", "TECHNITIUM_INSECURE": "yes"},
			wantErr: "Invalid TECHNITIUM_INSECURE",
		},
		"wrong password": {
			cfg:     map[string]tftypes.Value{"username": str("admin"), "password": str("wrong")},
			wantErr: "Login failed",
		},
	} {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("TECHNITIUM_HOST", srv.URL)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			resp := configure(t, tc.cfg)
			if tc.wantErr == "" {
				require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
				assert.NotNil(t, resp.ResourceData)
				return
			}
			require.True(t, resp.Diagnostics.HasError())
			assert.Equal(t, tc.wantErr, resp.Diagnostics.Errors()[0].Summary())
		})
	}
}
