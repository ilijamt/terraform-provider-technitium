package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
)

type Resource struct {
	spec   Spec
	client API
}

var (
	_ resource.ResourceWithConfigure   = (*Resource)(nil)
	_ resource.ResourceWithImportState = (*Resource)(nil)
)

func NewResource(spec Spec) func() resource.Resource {
	return func() resource.Resource { return &Resource{spec: spec} }
}

func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.spec.Name
}

func (r *Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(API)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected framework.API, got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "The resource identifier.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for _, a := range r.spec.Attrs {
		attrs[a.Name] = r.attribute(a)
	}
	resp.Schema = schema.Schema{Description: r.spec.Description, Attributes: attrs}
}

func (r *Resource) attribute(a Attr) schema.Attribute {
	required, optional, computed := a.Required, !a.Required && !a.ReadOnly, a.Computed()
	// A read-only value may change on any update, so it is not kept from state.
	keep := computed && !a.ReadOnly
	// A value the server does not return is unknown after import, so only
	// a change away from a known value replaces the resource. A singleton is
	// replaced only when it is moved to another object.
	replace := a.ForceNew() && (!r.spec.Singleton() || a.Identity)
	readable := a.Key != ""
	const replaceDesc = "Changing this replaces the resource."

	switch a.Type {
	case Bool:
		var pm []planmodifier.Bool
		if replace {
			pm = append(pm, boolplanmodifier.RequiresReplaceIf(func(_ context.Context, req planmodifier.BoolRequest, resp *boolplanmodifier.RequiresReplaceIfFuncResponse) {
				resp.RequiresReplace = readable || !req.StateValue.IsNull()
			}, replaceDesc, replaceDesc))
		}
		if keep {
			pm = append(pm, boolplanmodifier.UseStateForUnknown())
		}
		return schema.BoolAttribute{Required: required, Optional: optional, Computed: computed, Sensitive: a.Sensitive, Description: a.Description, PlanModifiers: pm}
	case Int64:
		var pm []planmodifier.Int64
		if replace {
			pm = append(pm, int64planmodifier.RequiresReplaceIf(func(_ context.Context, req planmodifier.Int64Request, resp *int64planmodifier.RequiresReplaceIfFuncResponse) {
				resp.RequiresReplace = readable || !req.StateValue.IsNull()
			}, replaceDesc, replaceDesc))
		}
		if keep {
			pm = append(pm, int64planmodifier.UseStateForUnknown())
		}
		return schema.Int64Attribute{Required: required, Optional: optional, Computed: computed, Sensitive: a.Sensitive, Description: a.Description, PlanModifiers: pm}
	case Table:
		var pm []planmodifier.Set
		if replace {
			pm = append(pm, setplanmodifier.RequiresReplaceIf(func(_ context.Context, req planmodifier.SetRequest, resp *setplanmodifier.RequiresReplaceIfFuncResponse) {
				resp.RequiresReplace = readable || !req.StateValue.IsNull()
			}, replaceDesc, replaceDesc))
		}
		if keep {
			pm = append(pm, setplanmodifier.UseStateForUnknown())
		}
		return schema.SetNestedAttribute{NestedObject: schema.NestedAttributeObject{Attributes: columns(a.Columns)},
			Required: required, Optional: optional, Computed: computed, Sensitive: a.Sensitive, Description: a.Description, PlanModifiers: pm}
	case Map:
		var pm []planmodifier.Map
		if replace {
			pm = append(pm, mapplanmodifier.RequiresReplaceIf(func(_ context.Context, req planmodifier.MapRequest, resp *mapplanmodifier.RequiresReplaceIfFuncResponse) {
				resp.RequiresReplace = readable || !req.StateValue.IsNull()
			}, replaceDesc, replaceDesc))
		}
		if keep {
			pm = append(pm, mapplanmodifier.UseStateForUnknown())
		}
		return schema.MapAttribute{ElementType: types.StringType, Required: required, Optional: optional, Computed: computed, Sensitive: a.Sensitive, Description: a.Description, PlanModifiers: pm}
	case List:
		var pm []planmodifier.List
		if replace {
			pm = append(pm, listplanmodifier.RequiresReplaceIf(func(_ context.Context, req planmodifier.ListRequest, resp *listplanmodifier.RequiresReplaceIfFuncResponse) {
				resp.RequiresReplace = readable || !req.StateValue.IsNull()
			}, replaceDesc, replaceDesc))
		}
		if keep {
			pm = append(pm, listplanmodifier.UseStateForUnknown())
		}
		return schema.ListAttribute{ElementType: types.StringType, Required: required, Optional: optional, Computed: computed, Sensitive: a.Sensitive, Description: a.Description, PlanModifiers: pm}
	}

	var pm []planmodifier.String
	if replace {
		pm = append(pm, stringplanmodifier.RequiresReplaceIf(func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = readable || !req.StateValue.IsNull()
		}, replaceDesc, replaceDesc))
	}
	if keep {
		pm = append(pm, stringplanmodifier.UseStateForUnknown())
	}
	v := pattern(a.Pattern)
	if len(a.Enum) > 0 {
		v = append(v, stringvalidator.OneOf(a.Enum...))
	}
	return schema.StringAttribute{Required: required, Optional: optional, Computed: computed, Sensitive: a.Sensitive, Description: a.Description, PlanModifiers: pm, Validators: v}
}

// columns returns the attributes of a table row; every cell is required.
func columns(cols []Column) map[string]schema.Attribute {
	out := map[string]schema.Attribute{}
	for _, c := range cols {
		switch c.Type {
		case Bool:
			out[c.Name] = schema.BoolAttribute{Required: true, Sensitive: c.Sensitive, Description: c.Description}
		case Int64:
			out[c.Name] = schema.Int64Attribute{Required: true, Sensitive: c.Sensitive, Description: c.Description}
		case List:
			out[c.Name] = schema.ListAttribute{ElementType: types.StringType, Required: true, Sensitive: c.Sensitive, Description: c.Description}
		default:
			out[c.Name] = schema.StringAttribute{Required: true, Sensitive: c.Sensitive, Description: c.Description, Validators: pattern(c.Pattern)}
		}
	}
	return out
}

func pattern(p string) []validator.String {
	if p == "" {
		return nil
	}
	return []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(p), "must match "+p)}
}

func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	plan, err := values(req.Plan.Raw)
	if err == nil {
		err = r.create(ctx, plan)
	}
	if err != nil {
		resp.Diagnostics.AddError("Create "+r.spec.Name, err.Error())
		return
	}

	// The object exists now, so later failures still save state and
	// Terraform taints it instead of losing track of it.
	state, err := r.read(ctx, plan)
	// Attributes only the update call accepts are applied in a second step.
	if state != nil && r.spec.Create != "" && r.hasUpdateOnly(plan) {
		if err = r.update(ctx, state, plan); err == nil {
			state, err = r.read(ctx, plan)
		}
	}
	if err == nil && state == nil {
		err = fmt.Errorf("%s was created but cannot be read back", r.spec.Name)
	}
	if state == nil {
		state = r.complete(plan)
	}
	setState(&resp.State, state)
	if err != nil {
		resp.Diagnostics.AddError("Create "+r.spec.Name, err.Error())
	}
}

func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	prior, err := values(req.State.Raw)
	var state map[string]tftypes.Value
	if err == nil {
		state, err = r.read(ctx, prior)
	}
	if err != nil {
		resp.Diagnostics.AddError("Read "+r.spec.Name, err.Error())
		return
	}
	if state == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	setState(&resp.State, state)
}

func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	plan, err := values(req.Plan.Raw)
	var prior, state map[string]tftypes.Value
	if err == nil {
		prior, err = values(req.State.Raw)
	}
	if err == nil {
		err = r.update(ctx, prior, plan)
	}
	if err == nil {
		state, err = r.read(ctx, plan)
		if err == nil && state == nil {
			err = fmt.Errorf("%s disappeared during update", r.spec.Name)
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Update "+r.spec.Name, err.Error())
		return
	}
	setState(&resp.State, state)
}

func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.spec.Singleton() {
		return
	}
	state, err := values(req.State.Raw)
	var params url.Values
	if err == nil {
		params, err = r.params(state, func(a Attr) bool { return a.Identity })
	}
	if err == nil {
		if _, err = r.client.Call(ctx, r.spec.Delete, params); client.IsNotFound(err) {
			err = nil
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Delete "+r.spec.Name, err.Error())
	}
}

// ImportState takes the identity values joined by "/", in schema order. The
// last one may itself contain "/"; an empty one is left unset. An ID starting
// with "{" is instead a JSON object of identity attribute names to values, for
// values that contain "/" elsewhere.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	ids := r.spec.identity()
	raws, err := importValues(req.ID, ids)
	if err != nil {
		resp.Diagnostics.AddError("Import "+r.spec.Name, err.Error())
		return
	}
	state := map[string]tftypes.Value{}
	for _, a := range ids {
		v, err := a.fromJSON(raws[a.Name])
		if err != nil {
			resp.Diagnostics.AddError("Import "+r.spec.Name, fmt.Sprintf("%s: %s", a.Name, err))
			return
		}
		state[a.Name] = v
	}
	setState(&resp.State, r.complete(state))
}

func importValues(id string, ids []Attr) (map[string]any, error) {
	raws := map[string]any{}
	if strings.HasPrefix(strings.TrimSpace(id), "{") {
		d := json.NewDecoder(strings.NewReader(id))
		d.UseNumber()
		if err := d.Decode(&raws); err != nil {
			return nil, fmt.Errorf("invalid JSON import ID: %w", err)
		}
		for k := range raws {
			if !slices.ContainsFunc(ids, func(a Attr) bool { return a.Name == k }) {
				return nil, fmt.Errorf("%s is not an identity attribute", k)
			}
		}
		return raws, nil
	}
	parts := strings.SplitN(id, "/", max(len(ids), 1))
	if len(parts) != len(ids) && len(ids) > 0 {
		return nil, fmt.Errorf("expected %d values separated by \"/\", got %q", len(ids), id)
	}
	for i, a := range ids {
		if parts[i] != "" {
			raws[a.Name] = parts[i]
		}
	}
	return raws, nil
}

func (r *Resource) create(ctx context.Context, plan map[string]tftypes.Value) error {
	if r.spec.Adopt {
		current, err := r.read(ctx, plan)
		if err == nil && current == nil {
			err = fmt.Errorf("%s does not exist", r.spec.Name)
		}
		if err != nil {
			return err
		}
		return r.update(ctx, current, plan)
	}
	endpoint, accepts := r.spec.Create, func(a Attr) bool { return a.Create }
	if endpoint == "" {
		endpoint, accepts = r.spec.Update, func(a Attr) bool { return a.Update }
	}
	params, err := r.params(plan, func(a Attr) bool { return a.Identity || accepts(a) })
	if err != nil {
		return err
	}
	if endpoint == r.spec.Create {
		for k, v := range r.spec.CreateFixed {
			params.Set(k, v)
		}
		for _, a := range r.spec.Attrs {
			if a.CreateParam != "" && params.Has(a.Param) {
				params[a.CreateParam] = params[a.Param]
				params.Del(a.Param)
			}
		}
	}
	_, err = r.client.Call(ctx, endpoint, params)
	return err
}

// update sends the identity and every updatable attribute that changed from
// prior. Unknown planned values fall back to prior.
func (r *Resource) update(ctx context.Context, prior, plan map[string]tftypes.Value) error {
	params, err := r.params(prior, func(a Attr) bool { return a.Identity })
	if err != nil {
		return err
	}
	changed := false
	for _, a := range r.spec.Attrs {
		if !a.Update || a.Identity {
			continue
		}
		v := plan[a.Name]
		if !v.IsKnown() {
			v = prior[a.Name]
		}
		if !set(v) {
			continue
		}
		changed = changed || !v.Equal(prior[a.Name])
		s, err := a.param(v)
		if err != nil {
			return fmt.Errorf("%s: %w", a.Name, err)
		}
		params.Set(a.Param, s)
	}
	if !changed {
		return nil
	}
	_, err = r.client.Call(ctx, r.spec.Update, params)
	return err
}

// read fetches the object located by the identity in prior. It returns nil
// when the object no longer exists.
func (r *Resource) read(ctx context.Context, prior map[string]tftypes.Value) (map[string]tftypes.Value, error) {
	params, err := r.params(prior, func(a Attr) bool { return a.Identity })
	if err != nil {
		return nil, err
	}
	obj, err := r.client.Call(ctx, r.spec.Read, params)
	if client.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if r.spec.List != "" {
		if obj = r.find(obj, prior); obj == nil {
			return nil, nil
		}
	}

	state := map[string]tftypes.Value{}
	for _, a := range r.spec.Attrs {
		state[a.Name] = prior[a.Name]
		if a.Key == "" {
			continue
		}
		if raw, ok := lookup(obj, a.Key); ok {
			if v, err := a.fromJSON(raw); err == nil && !equivalent(a, v, prior[a.Name]) {
				state[a.Name] = v
			}
		}
	}
	return r.complete(state), nil
}

// find picks the list item matching Fixed and the identity values.
func (r *Resource) find(resp map[string]any, prior map[string]tftypes.Value) map[string]any {
	type want struct {
		attr  Attr
		value string
	}
	var wants []want
	for _, a := range r.spec.Attrs {
		if a.Identity && a.Key != "" && set(prior[a.Name]) {
			s, _ := a.param(prior[a.Name])
			wants = append(wants, want{a, s})
		}
	}

	items, _ := resp[r.spec.List].([]any)
next:
	for _, it := range items {
		item, ok := it.(map[string]any)
		if !ok {
			continue
		}
		for k, want := range r.spec.Fixed {
			if got, ok := item[k]; ok && fmt.Sprint(got) != want {
				continue next
			}
		}
		for _, w := range wants {
			raw, ok := lookup(item, w.attr.Key)
			if !ok {
				continue
			}
			got, err := w.attr.fromJSON(raw)
			if err != nil {
				continue next
			}
			if g, _ := w.attr.param(got); !sameName(g, w.value) {
				continue next
			}
		}
		return item
	}
	return nil
}

// params collects Fixed and the selected attributes that have a value.
func (r *Resource) params(vals map[string]tftypes.Value, include func(Attr) bool) (url.Values, error) {
	params := url.Values{}
	for k, v := range r.spec.Fixed {
		params.Set(k, v)
	}
	for _, a := range r.spec.Attrs {
		if !include(a) || !set(vals[a.Name]) {
			continue
		}
		s, err := a.param(vals[a.Name])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", a.Name, err)
		}
		params.Set(a.Param, s)
	}
	return params, nil
}

func (r *Resource) hasUpdateOnly(plan map[string]tftypes.Value) bool {
	for _, a := range r.spec.Attrs {
		if a.Update && !a.Create && !a.Identity && set(plan[a.Name]) {
			return true
		}
	}
	return false
}

// complete fills every attribute, turns unknowns into nulls and sets the id.
func (r *Resource) complete(state map[string]tftypes.Value) map[string]tftypes.Value {
	var id []string
	for _, a := range r.spec.Attrs {
		if v, ok := state[a.Name]; !ok || !v.IsKnown() {
			state[a.Name] = tftypes.NewValue(a.tf(), nil)
		}
		if a.Identity {
			s := ""
			if set(state[a.Name]) {
				s, _ = a.param(state[a.Name])
			}
			id = append(id, s)
		}
	}
	if len(id) == 0 {
		id = []string{r.spec.Name}
	}
	state["id"] = tftypes.NewValue(tftypes.String, strings.Join(id, "/"))
	return state
}

// equivalent reports whether the server value got only differs from prior in
// how the server normalizes it, so prior is kept: null for an empty list or
// set, and identity names without the trailing dot or in another case, as find
// matches them. Other strings keep the server value so case-only drift still
// shows.
func equivalent(a Attr, got, prior tftypes.Value) bool {
	if !set(prior) {
		return false
	}
	if got.Type().Is(tftypes.String) {
		if !a.Identity {
			return false
		}
		var g, p string
		return set(got) && got.As(&g) == nil && prior.As(&p) == nil && sameName(g, p)
	}
	var g, p []tftypes.Value
	return (got.Type().Is(tftypes.List{}) || got.Type().Is(tftypes.Set{})) &&
		(got.IsNull() || got.As(&g) == nil && len(g) == 0) &&
		(prior.As(&p) == nil && len(p) == 0)
}

func values(v tftypes.Value) (map[string]tftypes.Value, error) {
	m := map[string]tftypes.Value{}
	return m, v.As(&m)
}

func setState(s *tfsdk.State, state map[string]tftypes.Value) {
	s.Raw = tftypes.NewValue(s.Schema.Type().TerraformType(context.Background()), state)
}
