package technitium_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNSTTLDescription(t *testing.T) {
	srv, err := factories["technitium"]()
	require.NoError(t, err)
	resp, err := srv.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	require.NoError(t, err)
	for _, a := range resp.ResourceSchemas["technitium_record_ns"].Block.Attributes {
		if a.Name == "ttl" {
			assert.Contains(t, a.Description, "default NS record TTL")
		}
	}
}
