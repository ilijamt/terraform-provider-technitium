// Package provider configures the Technitium API client.
package provider

import (
	"context"
	"net/http"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
)

type Provider struct {
	version    string
	httpClient *http.Client
	resources  []func() resource.Resource
}

type model struct {
	Host     types.String `tfsdk:"host"`
	Token    types.String `tfsdk:"token"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	Insecure types.Bool   `tfsdk:"insecure"`
}

func New(version string, httpClient *http.Client, resources []func() resource.Resource) func() provider.Provider {
	return func() provider.Provider {
		return &Provider{version: version, httpClient: httpClient, resources: resources}
	}
}

func (p *Provider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "technitium"
	resp.Version = p.version
}

func (p *Provider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Technitium DNS Server through its HTTP API.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Optional:    true,
				Description: "API address, e.g. http://localhost:5380. Defaults to TECHNITIUM_HOST.",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "API token. Defaults to TECHNITIUM_TOKEN. Conflicts with username and password.",
			},
			"username": schema.StringAttribute{
				Optional:    true,
				Description: "User to log in as when no token is set. Defaults to TECHNITIUM_USERNAME.",
			},
			"password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Password for username. Defaults to TECHNITIUM_PASSWORD.",
			},
			"insecure": schema.BoolAttribute{
				Optional:    true,
				Description: "Skip TLS certificate verification. Defaults to TECHNITIUM_INSECURE, or false.",
			},
		},
	}
}

func (p *Provider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var m model
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for name, v := range map[string]attr.Value{"host": m.Host, "token": m.Token, "username": m.Username, "password": m.Password, "insecure": m.Insecure} {
		if v.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Unknown "+name, "The provider cannot be configured while "+name+" is unknown. Use a value known at plan time.")
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	host := orEnv(m.Host.ValueString(), "TECHNITIUM_HOST")
	token, username, password := m.Token.ValueString(), m.Username.ValueString(), m.Password.ValueString()
	switch {
	case token != "":
	case username != "" || password != "":
		username = orEnv(username, "TECHNITIUM_USERNAME")
		password = orEnv(password, "TECHNITIUM_PASSWORD")
	default:
		token, username, password = os.Getenv("TECHNITIUM_TOKEN"), os.Getenv("TECHNITIUM_USERNAME"), os.Getenv("TECHNITIUM_PASSWORD")
	}
	insecure := m.Insecure.ValueBool()
	if v := os.Getenv("TECHNITIUM_INSECURE"); m.Insecure.IsNull() && v != "" {
		var err error
		if insecure, err = strconv.ParseBool(v); err != nil {
			resp.Diagnostics.AddError("Invalid TECHNITIUM_INSECURE", err.Error())
			return
		}
	}

	switch {
	case host == "":
		resp.Diagnostics.AddError("Missing host", "Set host or TECHNITIUM_HOST.")
	case token != "" && (username != "" || password != ""):
		resp.Diagnostics.AddError("Conflicting credentials", "Set either token, or username and password, not both.")
	case token == "" && (username == "" || password == ""):
		resp.Diagnostics.AddError("Missing credentials", "Set token, or username and password.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	c := client.New(host, token, "terraform-provider-technitium/"+p.version, insecure, p.httpClient)
	if token == "" {
		if err := c.Login(ctx, username, password); err != nil {
			resp.Diagnostics.AddError("Login failed", err.Error())
			return
		}
	}
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *Provider) Resources(context.Context) []func() resource.Resource { return p.resources }

func (p *Provider) DataSources(context.Context) []func() datasource.DataSource { return nil }

func orEnv(v, env string) string {
	if v != "" {
		return v
	}
	return os.Getenv(env)
}
