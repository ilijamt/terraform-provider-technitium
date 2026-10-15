package technitium

import (
	"context"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dnssecAPI map[string]any

func (a dnssecAPI) Call(_ context.Context, path string, _ url.Values) (map[string]any, error) {
	if path == "/api/zones/dnssec/properties/get" {
		return a, nil
	}
	return map[string]any{}, nil
}

func TestReadDNSSECKeySizes(t *testing.T) {
	key := func(typ, alg, state string) map[string]any {
		return map[string]any{"keyType": typ, "algorithm": alg, "state": state}
	}
	info, err := readDNSSEC(context.Background(), dnssecAPI{"dnssecStatus": "SignedWithNSEC", "dnssecPrivateKeys": []any{
		key("KeySigningKey", "RSASHA256 (2048 bits)", "Active"),
		key("KeySigningKey", "RSASHA256 (4096 bits)", "Generated"),
		key("ZoneSigningKey", "RSASHA256 (1024 bits)", "Published"),
		key("ZoneSigningKey", "RSASHA256 (1536 bits)", "Ready"),
	}}, "example.com")
	require.NoError(t, err)
	assert.Equal(t, types.Int64Value(2048), info.kskSize, "the active key")
	assert.Equal(t, types.Int64Value(1536), info.zskSize, "else the last one")

	info, err = readDNSSEC(context.Background(), dnssecAPI{"dnssecStatus": "SignedWithNSEC", "dnssecPrivateKeys": []any{
		key("KeySigningKey", "ECDSAP256SHA256", "Active"),
	}}, "example.com")
	require.NoError(t, err)
	assert.True(t, info.kskSize.IsNull())
	assert.True(t, info.zskSize.IsNull())
}
