package technitium_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/provider"
	"github.com/ilijamt/terraform-provider-technitium/internal/technitium"
)

var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"technitium": providerserver.NewProtocol6WithError(provider.New("test", nil, technitium.Resources(), technitium.DataSources())()),
}

func TestSchemas(t *testing.T) {
	srv, err := factories["technitium"]()
	require.NoError(t, err)
	resp, err := srv.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	require.NoError(t, err)
	for _, d := range resp.Diagnostics {
		t.Errorf("%s: %s", d.Summary, d.Detail)
	}
	assert.Len(t, resp.ResourceSchemas, len(technitium.Resources()))
	assert.Len(t, resp.DataSourceSchemas, len(technitium.DataSources()))
	for _, name := range []string{"technitium_group", "technitium_permissions_zones", "technitium_settings_general", "technitium_zone"} {
		assert.Contains(t, resp.DataSourceSchemas, name)
	}
	assert.NotContains(t, resp.DataSourceSchemas, "technitium_record_a")
	for _, name := range []string{"technitium_settings_general", "technitium_zone", "technitium_record_a", "technitium_record_fwd"} {
		assert.Contains(t, resp.ResourceSchemas, name)
	}
}

func TestSpecNamesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range technitium.Specs() {
		assert.False(t, seen[s.Name], "%s is defined twice", s.Name)
		seen[s.Name] = true
	}
}
