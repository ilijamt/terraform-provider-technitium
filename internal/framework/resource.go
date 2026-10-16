package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
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
	_ resource.ResourceWithModifyPlan  = (*Resource)(nil)
)

func NewResource(spec Spec) func() resource.Resource {
	spec.Attrs = slices.DeleteFunc(slices.Clone(spec.Attrs), func(a Attr) bool { return a.DataOnly })
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
	keep := computed && !a.ReadOnly && a.Default == nil
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
		attr := schema.BoolAttribute{Required: required, Optional: optional, Computed: computed, Sensitive: a.Sensitive, Description: a.Description, PlanModifiers: pm}
		if d, ok := a.Default.(bool); ok {
			attr.Default = booldefault.StaticBool(d)
		}
		return attr
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
		attr := schema.Int64Attribute{Required: required, Optional: optional, Computed: computed, Sensitive: a.Sensitive, Description: a.Description, PlanModifiers: pm}
		if d, ok := a.Default.(int); ok {
			attr.Default = int64default.StaticInt64(int64(d))
		}
		return attr
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
		var v []validator.Set
		if len(a.Kept) > 0 {
			v = append(v, keptValidator{a.Kept})
		}
		return schema.SetNestedAttribute{NestedObject: schema.NestedAttributeObject{Attributes: columns(a.Columns)},
			Required: required, Optional: optional, Computed: computed, Sensitive: a.Sensitive, Description: a.Description, PlanModifiers: pm, Validators: v}
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
			resp.RequiresReplace = (readable || !req.StateValue.IsNull()) &&
				!a.Convert.allows(req.StateValue.ValueString(), req.PlanValue.ValueString())
		}, replaceDesc, replaceDesc))
	}
	if keep {
		pm = append(pm, stringplanmodifier.UseStateForUnknown())
	}
	v := append(pattern(a.Pattern), a.Validators...)
	if a.compare() == IPAddress {
		v = append(v, ipValidator{})
	}
	if len(a.Enum) > 0 {
		v = append(v, stringvalidator.OneOf(a.Enum...))
	}
	attr := schema.StringAttribute{Required: required, Optional: optional, Computed: computed, Sensitive: a.Sensitive, Description: a.Description, PlanModifiers: pm, Validators: v}
	if d, ok := a.Default.(string); ok {
		attr.Default = stringdefault.StaticString(d)
	}
	if a.JSON {
		attr.CustomType = jsontypes.NormalizedType{}
	}
	return attr
}

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
	if client.IsMoved(err) {
		state, err = r.unread(plan, err, &resp.Diagnostics, "Create"), nil
	}
	if state != nil && r.spec.Create != "" && r.hasUpdateOnly(plan) {
		if err = r.update(ctx, state, plan); err == nil {
			state, err = r.read(ctx, plan)
		}
	}
	if err == nil && state == nil {
		err = fmt.Errorf("%s was created but cannot be read back", r.spec.Name)
	}
	if err == nil {
		err = r.missing(plan, state)
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
		if client.IsMoved(err) {
			state, err = r.unread(plan, err, &resp.Diagnostics, "Update"), nil
		}
		if err == nil && state == nil {
			err = fmt.Errorf("%s disappeared during update", r.spec.Name)
		}
	}
	if err == nil {
		// The state read back, so the next plan sends the names again.
		setState(&resp.State, state)
		err = r.missing(plan, state)
	}
	if err != nil {
		resp.Diagnostics.AddError("Update "+r.spec.Name, err.Error())
	}
}

// unread is the state of a change the server took that cannot be read back,
// as it moved the web service away from the provider's host.
func (r *Resource) unread(plan map[string]tftypes.Value, err error, diags *diag.Diagnostics, op string) map[string]tftypes.Value {
	diags.AddWarning(op+" "+r.spec.Name, "Saved as planned without reading it back: "+err.Error())
	return r.complete(plan)
}

func (r *Resource) ModifyPlan(_ context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	plan, err := values(req.Plan.Raw)
	var prior, config map[string]tftypes.Value
	if err == nil && !req.State.Raw.IsNull() {
		prior, err = values(req.State.Raw)
	}
	if err == nil && r.spec.Check != nil {
		err = r.spec.Check(prior, plan)
	}
	if err == nil && prior == nil {
		return
	}
	if err == nil {
		config, err = values(req.Config.Raw)
	}
	if err != nil {
		resp.Diagnostics.AddError("Plan "+r.spec.Name, err.Error())
		return
	}
	reset := map[string]bool{}
	for _, a := range r.spec.Attrs {
		if !plan[a.Name].IsKnown() || !a.matches(plan[a.Name], prior[a.Name]) {
			for _, name := range a.Resets {
				reset[name] = true
			}
		}
	}
	converts := r.converts(prior, plan) != nil
	if !converts && len(reset) == 0 {
		return
	}
	for _, a := range r.spec.Attrs {
		resets := reset[a.Name] || converts && a.Computed() && !a.Identity && a.Convert == nil && a.Default == nil
		if resets && config[a.Name].IsNull() {
			plan[a.Name] = tftypes.NewValue(a.tf(), tftypes.UnknownValue)
		}
	}
	resp.Plan.Raw = tftypes.NewValue(req.Plan.Raw.Type(), plan)
}

func (r *Resource) converts(prior, plan map[string]tftypes.Value) []Attr {
	var out []Attr
	for _, a := range r.spec.Attrs {
		var from, to string
		if a.Convert != nil && set(prior[a.Name]) && set(plan[a.Name]) &&
			prior[a.Name].As(&from) == nil && plan[a.Name].As(&to) == nil && a.Convert.allows(from, to) {
			out = append(out, a)
		}
	}
	return out
}

func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.spec.Singleton() {
		return
	}
	state, err := values(req.State.Raw)
	var params url.Values
	if err == nil {
		params, err = r.identify(ctx, state)
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

// ImportState takes the identity values joined by "/", in schema order, where
// only the last may contain "/". An ID starting with "{" is a JSON object of
// identity values instead.
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
		// The update call would take over an object Terraform does not manage.
		if !r.spec.Singleton() {
			current, err := r.read(ctx, plan)
			if err != nil {
				return err
			}
			if current != nil {
				return fmt.Errorf("%s already exists; import it to manage it", r.spec.Name)
			}
		}
	}
	if endpoint == r.spec.Create {
		if _, err := r.share(ctx, nil, plan); err != nil {
			return err
		}
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
			// The create call may refuse the default, such as an empty name.
			if d, ok := a.defaultValue(); ok && d.Equal(plan[a.Name]) {
				params.Del(a.Param)
			}
			if a.CreateParam != "" && params.Has(a.Param) {
				params[a.CreateParam] = params[a.Param]
				params.Del(a.Param)
			}
		}
	}
	cctx := ctx
	if endpoint == r.spec.Update && r.restarts(ctx, plan) {
		cctx = client.Restarts(ctx)
	}
	resp, err := r.client.Call(cctx, endpoint, params)
	// A create cut off mid-way may still have happened; the object then
	// exists. Not when create returns values only its response holds.
	if client.IsDropped(err) && r.spec.Created == nil {
		if state, rerr := r.read(ctx, plan); rerr == nil && state != nil {
			err = nil
		}
	}
	if err != nil {
		return err
	}
	if r.spec.Created != nil {
		for name, raw := range r.spec.Created(resp) {
			i := slices.IndexFunc(r.spec.Attrs, func(a Attr) bool { return a.Name == name })
			if i < 0 {
				return fmt.Errorf("created value for unknown attribute %s", name)
			}
			if plan[name], err = r.spec.Attrs[i].fromJSON(raw); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	return r.toggle(ctx, map[string]tftypes.Value{}, plan)
}

func (r *Resource) update(ctx context.Context, prior, plan map[string]tftypes.Value) error {
	moved, err := r.share(ctx, prior, plan)
	if err != nil {
		return err
	}
	params, err := r.identify(ctx, prior)
	if err != nil {
		return err
	}
	converts := r.converts(prior, plan)
	var live map[string]tftypes.Value
	if len(converts) > 0 {
		// Another conversion in this apply, such as of a catalog zone, may
		// have converted the object already.
		if live, err = r.read(ctx, prior); err != nil {
			return err
		}
	}
	for _, a := range converts {
		if live != nil && a.matches(live[a.Name], plan[a.Name]) {
			continue
		}
		p := maps.Clone(params)
		s, err := a.param(plan[a.Name])
		if err != nil {
			return fmt.Errorf("%s: %w", a.Name, err)
		}
		p.Set(a.Param, s)
		if _, err = r.client.Call(ctx, a.Convert.Path, p); err != nil {
			return err
		}
	}
	for _, a := range r.spec.Attrs {
		if a.NewParam == "" || !set(plan[a.Name]) {
			continue
		}
		s, err := a.param(plan[a.Name])
		if err != nil {
			return fmt.Errorf("%s: %w", a.Name, err)
		}
		params.Set(a.NewParam, s)
	}
	// A conversion resets every value, so all known ones are sent again.
	changed := len(converts) > 0
	restarts := false
	for _, a := range r.spec.Attrs {
		if !a.Update || a.Identity || a.Toggle != nil {
			continue
		}
		v := plan[a.Name]
		if !v.IsKnown() && len(converts) == 0 {
			v = prior[a.Name]
		}
		if !set(v) {
			continue
		}
		differs := !v.Equal(prior[a.Name])
		changed = changed || differs
		restarts = restarts || a.Restarts && differs
		if a.OnChange && !differs {
			continue
		}
		s, err := a.param(v)
		if err != nil {
			return fmt.Errorf("%s: %w", a.Name, err)
		}
		params.Set(a.Param, s)
	}
	if changed {
		uctx := ctx
		if r.spec.UpdateOnce {
			uctx = client.Once(ctx)
		}
		if restarts {
			uctx = client.Restarts(uctx)
		}
		if _, err = r.client.Call(uctx, r.spec.Update, params); err != nil {
			return err
		}
		if err = r.restore(ctx, moved, plan); err != nil {
			return err
		}
	}
	return r.toggle(ctx, prior, plan)
}

// restarts tells whether creating with the update call changes a Restarts
// value on the server. One the server never returns, or one it cannot be
// compared with, counts as changed.
func (r *Resource) restarts(ctx context.Context, plan map[string]tftypes.Value) bool {
	var current map[string]tftypes.Value
	for _, a := range r.spec.Attrs {
		if !a.Restarts || !set(plan[a.Name]) {
			continue
		}
		if a.Key == "" {
			return true
		}
		if current == nil {
			var err error
			if current, err = r.read(ctx, plan); err != nil || current == nil {
				return true
			}
		}
		if !a.matches(plan[a.Name], current[a.Name]) {
			return true
		}
	}
	return false
}

// share checks the Shared value of plan against the other List items, and
// returns them when the change moves them all with it.
func (r *Resource) share(ctx context.Context, prior, plan map[string]tftypes.Value) ([]map[string]any, error) {
	i := slices.IndexFunc(r.spec.Attrs, func(a Attr) bool { return a.Name == r.spec.Shared })
	if i < 0 || prior != nil && plan[r.spec.Shared].Equal(prior[r.spec.Shared]) {
		return nil, nil
	}
	a, self := r.spec.Attrs[i], plan
	if prior != nil {
		self = prior
	}
	params, err := r.params(self, func(a Attr) bool { return a.Identity })
	if err != nil {
		return nil, err
	}
	resp, err := r.client.Call(ctx, r.spec.Read, params)
	if client.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var others []map[string]any
	for _, item := range r.items(resp, r.spec.List) {
		if !r.same(item, self) {
			others = append(others, item)
		} else if prior == nil && r.located(plan) {
			// The add call would take it over with another TTL.
			return nil, fmt.Errorf("%s already exists; import it to manage it", r.spec.Name)
		}
	}
	if len(others) == 0 {
		return nil, nil
	}
	raw, _ := lookup(others[0], a.Key)
	want, err := a.fromJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", a.Name, err)
	}
	switch {
	case !set(plan[a.Name]):
		plan[a.Name] = want
		return nil, nil
	case plan[a.Name].Equal(want):
		return nil, nil
	case prior != nil && prior[a.Name].Equal(want):
		return others, nil
	}
	s, _ := a.param(want)
	return nil, fmt.Errorf("records at %v of type %v share one %s (%s); set %s = %s on every one",
		others[0]["name"], others[0]["type"], a.Name, s, a.Name, s)
}

// restore sends the values of items again with the planned Shared value,
// since the server drops what it does not keep in the record data when it
// moves them.
func (r *Resource) restore(ctx context.Context, items []map[string]any, plan map[string]tftypes.Value) error {
	for _, item := range items {
		vals := map[string]tftypes.Value{r.spec.Shared: plan[r.spec.Shared]}
		for _, a := range r.spec.Attrs {
			if a.Key == "" || a.Name == r.spec.Shared || a.ReadOnly || !a.Identity && !a.Update {
				continue
			}
			if raw, ok := lookup(item, a.Key); ok {
				if v, err := a.fromJSON(raw); err == nil {
					vals[a.Name] = v
				}
			} else if d, ok := a.defaultValue(); ok {
				vals[a.Name] = d
			}
		}
		for _, a := range r.spec.Attrs {
			if _, ok := vals[a.Name]; !ok && a.Identity {
				vals[a.Name] = plan[a.Name]
			}
		}
		params, err := r.params(vals, func(Attr) bool { return true })
		if err != nil {
			return err
		}
		if _, err = r.client.Call(client.Once(ctx), r.spec.Update, params); err != nil {
			return err
		}
	}
	return nil
}

func (r *Resource) toggle(ctx context.Context, prior, plan map[string]tftypes.Value) error {
	for _, a := range r.spec.Attrs {
		v := plan[a.Name]
		if a.Toggle == nil || !set(v) || v.Equal(prior[a.Name]) {
			continue
		}
		var on bool
		if err := v.As(&on); err != nil {
			return fmt.Errorf("%s: %w", a.Name, err)
		}
		endpoint := a.Toggle.Off
		if on {
			endpoint = a.Toggle.On
		}
		params, err := r.params(plan, func(a Attr) bool { return a.Identity })
		if err != nil {
			return err
		}
		if _, err = r.client.Call(ctx, endpoint, params); err != nil {
			return err
		}
	}
	return nil
}

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
		if obj = r.find(obj, r.spec.List, prior); obj == nil {
			return nil, nil
		}
	}

	sources := map[Source]map[string]any{}
	state := map[string]tftypes.Value{}
	for _, a := range r.spec.Attrs {
		state[a.Name] = prior[a.Name]
		if a.Key == "" {
			continue
		}
		from := obj
		if a.Source != nil {
			if _, ok := sources[*a.Source]; !ok {
				resp, err := r.client.Call(ctx, a.Source.Path, params)
				if err != nil {
					return nil, err
				}
				sources[*a.Source] = r.find(resp, a.Source.List, prior)
			}
			if from = sources[*a.Source]; from == nil {
				continue
			}
		}
		var v tftypes.Value
		// A null reads as the default too, as for a member's empty catalog.
		if raw, ok := lookup(from, a.Key); ok && (raw != nil || a.Default == nil) {
			if v, err = a.fromJSON(raw); err != nil {
				continue
			}
		} else if v, ok = a.defaultValue(); !ok {
			continue
		}
		if v = r.hideAuto(a, v, from, prior[a.Name]); !equivalent(a, v, prior[a.Name]) {
			state[a.Name] = dropKept(a, v, prior[a.Name])
		}
	}
	return r.complete(state), nil
}

func (r *Resource) find(resp map[string]any, list string, prior map[string]tftypes.Value) map[string]any {
	for _, item := range r.items(resp, list) {
		if r.same(item, prior) {
			return item
		}
	}
	return nil
}

// items are the List items of resp with the Fixed values.
func (r *Resource) items(resp map[string]any, list string) []map[string]any {
	var out []map[string]any
	raws, _ := resp[list].([]any)
next:
	for _, it := range raws {
		item, ok := it.(map[string]any)
		if !ok {
			continue
		}
		for k, want := range r.spec.Fixed {
			if got, ok := item[k]; ok && fmt.Sprint(got) != want {
				continue next
			}
		}
		out = append(out, item)
	}
	return out
}

// located is whether every identity value read back is known, so same
// matches one object only.
func (r *Resource) located(vals map[string]tftypes.Value) bool {
	for _, a := range r.spec.Attrs {
		if a.Identity && a.Key != "" && !set(vals[a.Name]) {
			return false
		}
	}
	return true
}

func (r *Resource) same(item map[string]any, vals map[string]tftypes.Value) bool {
	for _, a := range r.spec.Attrs {
		if !a.Identity || a.Key == "" || !set(vals[a.Name]) {
			continue
		}
		raw, ok := lookup(item, a.Key)
		if !ok {
			continue
		}
		got, err := a.fromJSON(raw)
		if err != nil || !a.matches(r.hideAuto(a, got, item, vals[a.Name]), vals[a.Name]) {
			return false
		}
	}
	return true
}

func (r *Resource) identify(ctx context.Context, vals map[string]tftypes.Value) (url.Values, error) {
	params, err := r.params(vals, func(a Attr) bool { return a.Identity })
	auto := func(a Attr) bool { return a.Identity && len(a.Auto) > 0 }
	if err != nil || r.spec.List == "" || !slices.ContainsFunc(r.spec.Attrs, auto) {
		return params, err
	}
	resp, err := r.client.Call(ctx, r.spec.Read, params)
	if client.IsNotFound(err) {
		return params, nil
	}
	if err != nil {
		return nil, err
	}
	item := r.find(resp, r.spec.List, vals)
	for _, a := range r.spec.Attrs {
		if item == nil || !auto(a) {
			continue
		}
		raw, ok := lookup(item, a.Key)
		if !ok {
			continue
		}
		v, err := a.fromJSON(raw)
		var s string
		if err == nil {
			s, err = a.param(v)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", a.Name, err)
		}
		params.Set(a.Param, s)
	}
	return params, nil
}

func (r *Resource) hideAuto(a Attr, got tftypes.Value, item map[string]any, prior tftypes.Value) tftypes.Value {
	var elems, before map[string]tftypes.Value
	if len(a.Auto) == 0 || !set(got) || got.As(&elems) != nil {
		return got
	}
	if set(prior) {
		_ = prior.As(&before)
	}
	for key, flag := range a.Auto {
		i := slices.IndexFunc(r.spec.Attrs, func(f Attr) bool { return f.Name == flag })
		if i < 0 {
			continue
		}
		if on, _ := lookup(item, r.spec.Attrs[i].Key); on != true {
			continue
		}
		delete(elems, key)
		if v, ok := before[key]; ok {
			elems[key] = v
		}
	}
	return tftypes.NewValue(got.Type(), elems)
}

func (r *Resource) params(vals map[string]tftypes.Value, include func(Attr) bool) (url.Values, error) {
	params := url.Values{}
	for k, v := range r.spec.Fixed {
		params.Set(k, v)
	}
	for _, a := range r.spec.Attrs {
		if !include(a) || a.Toggle != nil || !set(vals[a.Name]) {
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

func equivalent(a Attr, got, prior tftypes.Value) bool {
	if !set(prior) {
		return false
	}
	if got.Type().Is(tftypes.String) {
		var g, p string
		return set(got) && got.As(&g) == nil && prior.As(&p) == nil && a.compare().same(g, p)
	}
	if a.Type == Map && a.compare() == SvcParams {
		return svcParams(got, prior)
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
