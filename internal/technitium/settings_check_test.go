package technitium

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
)

func TestCheckLocalEndPoints(t *testing.T) {
	list := tftypes.List{ElementType: tftypes.String}
	check := func(v tftypes.Value) error {
		return checkLocalEndPoints(nil, map[string]tftypes.Value{"dns_server_local_end_points": v})
	}
	eps := func(s ...string) tftypes.Value {
		var out []tftypes.Value
		for _, e := range s {
			out = append(out, tftypes.NewValue(tftypes.String, e))
		}
		return tftypes.NewValue(list, out)
	}

	assert.NoError(t, check(tftypes.NewValue(list, nil)), "unset")
	assert.NoError(t, check(tftypes.NewValue(list, tftypes.UnknownValue)), "unknown")
	assert.NoError(t, check(eps("0.0.0.0:53", "[::]:5353")))
	assert.NoError(t, check(tftypes.NewValue(list, []tftypes.Value{tftypes.NewValue(tftypes.String, tftypes.UnknownValue)})), "unknown end point")
	assert.ErrorContains(t, check(tftypes.NewValue(list, []tftypes.Value{})), "empty list")
	assert.ErrorContains(t, check(eps("0.0.0.0:53", "[::]:0")), `"[::]:0" needs a port`)
	assert.ErrorContains(t, check(eps("0.0.0.0")), `"0.0.0.0" needs a port`)
}
