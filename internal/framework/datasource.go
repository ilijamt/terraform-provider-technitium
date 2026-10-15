package framework

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// DataSource reads an object of a Spec without managing it. The identity
// attributes are its arguments; the attributes the server returns are its
// results.
type DataSource struct {
	spec   Spec
	client API
}

var _ datasource.DataSourceWithConfigure = (*DataSource)(nil)

func NewDataSource(spec Spec) func() datasource.DataSource {
	return func() datasource.DataSource { return &DataSource{spec: spec} }
}

// attrs are the attributes of the data source: the identity, and what the
// server returns. Values only ever sent, like passwords, are left out.
func (d *DataSource) attrs() []Attr {
	var out []Attr
	for _, a := range d.spec.Attrs {
		if a.Identity || a.Key != "" {
			out = append(out, a)
		}
	}
	return out
}

func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.spec.Name
}

func (d *DataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(API)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected framework.API, got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	var ids []string
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true, Description: "The identifier, as the resource has it."},
	}
	for _, a := range d.attrs() {
		required, computed := a.Identity, !a.Identity
		if a.Identity {
			ids = append(ids, "`"+a.Name+"`")
		}
		switch a.Type {
		case Bool:
			attrs[a.Name] = schema.BoolAttribute{Required: required, Computed: computed, Sensitive: a.Sensitive, Description: a.Description}
		case Int64:
			attrs[a.Name] = schema.Int64Attribute{Required: required, Computed: computed, Sensitive: a.Sensitive, Description: a.Description}
		case List:
			attrs[a.Name] = schema.ListAttribute{ElementType: types.StringType, Required: required, Computed: computed, Sensitive: a.Sensitive, Description: a.Description}
		case Map:
			attrs[a.Name] = schema.MapAttribute{ElementType: types.StringType, Required: required, Computed: computed, Sensitive: a.Sensitive, Description: a.Description}
		case Table:
			cols := map[string]schema.Attribute{}
			for _, c := range a.Columns {
				switch c.Type {
				case Bool:
					cols[c.Name] = schema.BoolAttribute{Computed: true, Sensitive: c.Sensitive, Description: c.Description}
				case Int64:
					cols[c.Name] = schema.Int64Attribute{Computed: true, Sensitive: c.Sensitive, Description: c.Description}
				case List:
					cols[c.Name] = schema.ListAttribute{ElementType: types.StringType, Computed: true, Sensitive: c.Sensitive, Description: c.Description}
				default:
					cols[c.Name] = schema.StringAttribute{Computed: true, Sensitive: c.Sensitive, Description: c.Description}
				}
			}
			attrs[a.Name] = schema.SetNestedAttribute{NestedObject: schema.NestedAttributeObject{Attributes: cols},
				Required: required, Computed: computed, Sensitive: a.Sensitive, Description: a.Description}
		default:
			attrs[a.Name] = schema.StringAttribute{Required: required, Computed: computed, Sensitive: a.Sensitive, Description: a.Description}
		}
	}
	desc := fmt.Sprintf("Reads what `technitium_%s` manages, without managing it.", d.spec.Name)
	if len(ids) > 0 {
		desc = fmt.Sprintf("Reads what `technitium_%s` manages, without managing it, found by %s.", d.spec.Name, strings.Join(ids, " and "))
	}
	resp.Schema = schema.Schema{Description: desc, Attributes: attrs}
}

func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	config, err := values(req.Config.Raw)
	var state map[string]tftypes.Value
	if err == nil {
		state, err = (&Resource{spec: d.spec, client: d.client}).read(ctx, config)
	}
	if err == nil && state == nil {
		err = fmt.Errorf("not found")
	}
	if err != nil {
		resp.Diagnostics.AddError("Read "+d.spec.Name, err.Error())
		return
	}
	out := map[string]tftypes.Value{"id": state["id"]}
	for _, a := range d.attrs() {
		out[a.Name] = state[a.Name]
	}
	resp.State.Raw = tftypes.NewValue(resp.State.Schema.Type().TerraformType(ctx), out)
}
