package framework

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
)

func TestDelete(t *testing.T) {
	for name, tc := range map[string]struct {
		err     error
		wantErr bool
	}{
		"deleted":   {},
		"not found": {err: &client.Error{Message: "No such zone was found: example.com"}},
		"denied":    {err: &client.Error{Message: "Access was denied."}, wantErr: true},
		"transport": {err: errors.New("boom"), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			api := &mockAPI{respond: func(string, url.Values) (map[string]any, error) { return map[string]any{}, tc.err }}
			r := &Resource{spec: recordA, client: api}
			s := schemaOf(r)
			st := identity()
			st["id"] = str("example.com/www.example.com/10.0.0.2")

			resp := &resource.DeleteResponse{}
			r.Delete(context.Background(), resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: object(s, st)}}, resp)

			assert.Equal(t, tc.wantErr, resp.Diagnostics.HasError())
			assert.Equal(t, []string{"delete"}, api.paths())
		})
	}
}

func TestReadRemovesGoneResource(t *testing.T) {
	api := &mockAPI{respond: func(string, url.Values) (map[string]any, error) {
		return nil, &client.Error{Message: "No such zone was found: example.com"}
	}}
	r := &Resource{spec: recordA, client: api}
	s := schemaOf(r)
	st := identity()
	st["id"] = str("example.com/www.example.com/10.0.0.2")
	raw := object(s, st)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: raw}}, resp)

	assert.False(t, resp.Diagnostics.HasError())
	assert.True(t, resp.State.Raw.IsNull(), "a missing object is dropped from state")
}

func TestReadKeepsStateOnError(t *testing.T) {
	api := &mockAPI{respond: func(string, url.Values) (map[string]any, error) {
		return nil, &client.Error{Message: "Access was denied."}
	}}
	r := &Resource{spec: recordA, client: api}
	s := schemaOf(r)
	raw := object(s, identity())

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: raw}}, resp)

	assert.True(t, resp.Diagnostics.HasError())
	assert.False(t, resp.State.Raw.IsNull())
}

func dataSourceSchema(d *DataSource) dschema.Schema {
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	return resp.Schema
}

func TestDataSourceRead(t *testing.T) {
	respond := func(records ...any) func(string, url.Values) (map[string]any, error) {
		return func(string, url.Values) (map[string]any, error) { return map[string]any{"records": records}, nil }
	}
	config := func(d *DataSource) tfsdk.Config {
		s := dataSourceSchema(d)
		typ := s.Type().TerraformType(context.Background()).(tftypes.Object)
		vals := map[string]tftypes.Value{}
		for n, at := range typ.AttributeTypes {
			vals[n] = tftypes.NewValue(at, nil)
		}
		for n, v := range identity() {
			vals[n] = v
		}
		return tfsdk.Config{Schema: s, Raw: tftypes.NewValue(typ, vals)}
	}

	t.Run("found", func(t *testing.T) {
		d := &DataSource{spec: recordA, client: &mockAPI{respond: respond(record("10.0.0.2", 300))}}
		cfg := config(d)
		resp := &datasource.ReadResponse{State: tfsdk.State{Schema: cfg.Schema, Raw: tftypes.NewValue(cfg.Schema.Type().TerraformType(context.Background()), nil)}}
		d.Read(context.Background(), datasource.ReadRequest{Config: cfg}, resp)

		require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
		var id types.String
		var ttl types.Int64
		resp.State.GetAttribute(context.Background(), path.Root("id"), &id)
		resp.State.GetAttribute(context.Background(), path.Root("ttl"), &ttl)
		assert.Equal(t, "example.com/www.example.com/10.0.0.2", id.ValueString())
		assert.Equal(t, int64(300), ttl.ValueInt64())
	})

	t.Run("not found", func(t *testing.T) {
		d := &DataSource{spec: recordA, client: &mockAPI{respond: respond(record("10.0.0.1", 60))}}
		cfg := config(d)
		resp := &datasource.ReadResponse{State: tfsdk.State{Schema: cfg.Schema, Raw: tftypes.NewValue(cfg.Schema.Type().TerraformType(context.Background()), nil)}}
		d.Read(context.Background(), datasource.ReadRequest{Config: cfg}, resp)

		assert.True(t, resp.Diagnostics.HasError())
	})

	t.Run("api error", func(t *testing.T) {
		d := &DataSource{spec: recordA, client: &mockAPI{respond: func(string, url.Values) (map[string]any, error) {
			return nil, errors.New("boom")
		}}}
		cfg := config(d)
		resp := &datasource.ReadResponse{State: tfsdk.State{Schema: cfg.Schema, Raw: tftypes.NewValue(cfg.Schema.Type().TerraformType(context.Background()), nil)}}
		d.Read(context.Background(), datasource.ReadRequest{Config: cfg}, resp)

		assert.True(t, resp.Diagnostics.HasError())
	})
}

// A value the server does not return is null after an import, and replaces
// the resource only when it changes away from a known value. A value the
// server does return replaces it on any change.
func TestRequiresReplaceIf(t *testing.T) {
	r := &Resource{spec: recordA}
	s := schemaOf(r)
	ctx := context.Background()

	replaces := func(t *testing.T, attr string, state, plan tftypes.Value) bool {
		t.Helper()
		stateVals, planVals := identity(), identity()
		stateVals[attr], planVals[attr] = state, plan
		req := planmodifier.BoolRequest{}
		sreq := planmodifier.StringRequest{}
		switch m := s.Attributes[attr].(type) {
		case schema.BoolAttribute:
			req.State = tfsdk.State{Schema: s, Raw: object(s, stateVals)}
			req.Plan = tfsdk.Plan{Schema: s, Raw: object(s, planVals)}
			req.Path = path.Root(attr)
			req.StateValue = types.BoolNull()
			if !state.IsNull() {
				var b bool
				require.NoError(t, state.As(&b))
				req.StateValue = types.BoolValue(b)
			}
			var b bool
			require.NoError(t, plan.As(&b))
			req.PlanValue = types.BoolValue(b)
			req.ConfigValue = req.PlanValue
			for _, pm := range m.PlanModifiers {
				resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
				pm.PlanModifyBool(ctx, req, resp)
				if resp.RequiresReplace {
					return true
				}
			}
		case schema.StringAttribute:
			sreq.State = tfsdk.State{Schema: s, Raw: object(s, stateVals)}
			sreq.Plan = tfsdk.Plan{Schema: s, Raw: object(s, planVals)}
			sreq.Path = path.Root(attr)
			sreq.StateValue = types.StringNull()
			if !state.IsNull() {
				var v string
				require.NoError(t, state.As(&v))
				sreq.StateValue = types.StringValue(v)
			}
			var v string
			require.NoError(t, plan.As(&v))
			sreq.PlanValue = types.StringValue(v)
			sreq.ConfigValue = sreq.PlanValue
			for _, pm := range m.PlanModifiers {
				resp := &planmodifier.StringResponse{PlanValue: sreq.PlanValue}
				pm.PlanModifyString(ctx, sreq, resp)
				if resp.RequiresReplace {
					return true
				}
			}
		default:
			t.Fatalf("unexpected attribute type %T", m)
		}
		return false
	}

	t.Run("not returned by the server", func(t *testing.T) {
		assert.False(t, replaces(t, "ptr", tftypes.NewValue(tftypes.Bool, nil), boolean(true)), "null state, as after an import")
		assert.True(t, replaces(t, "ptr", boolean(false), boolean(true)), "change from a known value")
		assert.False(t, replaces(t, "ptr", boolean(true), boolean(true)), "no change")
	})

	t.Run("returned by the server", func(t *testing.T) {
		assert.True(t, replaces(t, "ip_address", str("10.0.0.2"), str("10.0.0.3")))
		assert.True(t, replaces(t, "ip_address", tftypes.NewValue(tftypes.String, nil), str("10.0.0.3")), "a null state still replaces")
	})
}
