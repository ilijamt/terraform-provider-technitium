package terraforming

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ilijamt/terraform-provider-technitium/internal/provider"
	"github.com/ilijamt/terraform-provider-technitium/internal/technitium"
)

// reader imports and reads objects through the provider, in process, the way
// Terraform does, so the values are exactly what an import puts in state.
type reader struct {
	srv     tfprotov6.ProviderServer
	schemas map[string]*tfprotov6.Schema
}

func newReader(ctx context.Context, version string, hc *http.Client, o Options) (*reader, error) {
	srv, err := providerserver.NewProtocol6WithError(provider.New(version, hc, technitium.Resources(), nil)())()
	if err != nil {
		return nil, err
	}
	schema, err := srv.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		return nil, err
	}
	if err = diagnostics(schema.Diagnostics); err != nil {
		return nil, err
	}

	typ := schema.Provider.ValueType()
	str := func(s string) tftypes.Value {
		if s == "" {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, s)
	}
	config, err := tfprotov6.NewDynamicValue(typ, tftypes.NewValue(typ, map[string]tftypes.Value{
		"host":     str(o.Host),
		"token":    str(o.Token),
		"username": str(o.Username),
		"password": str(o.Password),
		"insecure": tftypes.NewValue(tftypes.Bool, o.Insecure),
	}))
	if err != nil {
		return nil, err
	}
	configured, err := srv.ConfigureProvider(ctx, &tfprotov6.ConfigureProviderRequest{Config: &config})
	if err != nil {
		return nil, err
	}
	if err = diagnostics(configured.Diagnostics); err != nil {
		return nil, err
	}
	return &reader{srv: srv, schemas: schema.ResourceSchemas}, nil
}

// read imports the object with the ID and returns its state, or nil when the
// object is gone.
func (r *reader) read(ctx context.Context, typ, id string) (map[string]tftypes.Value, error) {
	name := "technitium_" + typ
	schema, ok := r.schemas[name]
	if !ok {
		return nil, fmt.Errorf("no resource %s", name)
	}
	imported, err := r.srv.ImportResourceState(ctx, &tfprotov6.ImportResourceStateRequest{TypeName: name, ID: id})
	if err != nil {
		return nil, err
	}
	if err = diagnostics(imported.Diagnostics); err != nil {
		return nil, err
	}
	if len(imported.ImportedResources) != 1 {
		return nil, fmt.Errorf("import returned %d objects", len(imported.ImportedResources))
	}
	read, err := r.srv.ReadResource(ctx, &tfprotov6.ReadResourceRequest{
		TypeName:     name,
		CurrentState: imported.ImportedResources[0].State,
		Private:      imported.ImportedResources[0].Private,
	})
	if err != nil {
		return nil, err
	}
	if err = diagnostics(read.Diagnostics); err != nil {
		return nil, err
	}
	if read.NewState == nil {
		return nil, nil
	}
	v, err := read.NewState.Unmarshal(schema.ValueType())
	if err != nil || v.IsNull() {
		return nil, err
	}
	state := map[string]tftypes.Value{}
	return state, v.As(&state)
}

func diagnostics(diags []*tfprotov6.Diagnostic) error {
	var msgs []string
	for _, d := range diags {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			msgs = append(msgs, strings.TrimSpace(d.Summary+": "+d.Detail))
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return errors.New(strings.Join(msgs, "; "))
}
