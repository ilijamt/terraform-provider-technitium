package technitium

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
)

func TestDNSSECValidateUnknown(t *testing.T) {
	ctx := context.Background()
	r := &zoneDNSSEC{}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	typ := s.Schema.Type().TerraformType(ctx).(tftypes.Object)

	valid := func(set map[string]tftypes.Value) bool {
		vals := map[string]tftypes.Value{}
		for name, at := range typ.AttributeTypes {
			vals[name] = tftypes.NewValue(at, nil)
		}
		for name, v := range set {
			vals[name] = v
		}
		var resp resource.ValidateConfigResponse
		r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: s.Schema, Raw: tftypes.NewValue(typ, vals)}}, &resp)
		return !resp.Diagnostics.HasError()
	}
	alg := func(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
	size := func(n int64) tftypes.Value { return tftypes.NewValue(tftypes.Number, n) }
	unknownSize := tftypes.NewValue(tftypes.Number, tftypes.UnknownValue)
	unknownAlg := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)

	assert.True(t, valid(map[string]tftypes.Value{"algorithm": alg("ECDSA"), "ksk_key_size": unknownSize}), "a key size from a variable")
	assert.True(t, valid(map[string]tftypes.Value{"algorithm": unknownAlg, "ksk_key_size": size(2048)}), "an algorithm from a variable")
	assert.False(t, valid(map[string]tftypes.Value{"algorithm": alg("ECDSA"), "zsk_key_size": size(1024)}))
	assert.True(t, valid(map[string]tftypes.Value{"algorithm": alg("RSA"), "zsk_key_size": size(1024)}))
}
