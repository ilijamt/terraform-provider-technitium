package framework

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The rows of a permissions table are a name and the rights can_view,
// can_modify and can_delete. Kept names are rows the server always keeps
// with every right, whatever is set.
var rights = []string{"can_view", "can_modify", "can_delete"}

// dropKept removes from got the rows of kept names with every right, unless
// prior has a row for that name: the server adds them on its own, so a
// configuration may leave them out.
func dropKept(a Attr, got, prior tftypes.Value) tftypes.Value {
	var rows []tftypes.Value
	if len(a.Kept) == 0 || !set(got) || got.As(&rows) != nil {
		return got
	}
	listed := map[string]bool{}
	var priorRows []tftypes.Value
	if set(prior) && prior.As(&priorRows) == nil {
		for _, row := range priorRows {
			if name, _, ok := keptRow(row); ok {
				listed[name] = true
			}
		}
	}
	var out []tftypes.Value
	for _, row := range rows {
		name, full, ok := keptRow(row)
		if ok && full && slices.Contains(a.Kept, name) && !listed[name] {
			continue
		}
		out = append(out, row)
	}
	return tftypes.NewValue(got.Type(), out)
}

// keptRow returns the name of a row and whether it has every right.
func keptRow(row tftypes.Value) (string, bool, bool) {
	var obj map[string]tftypes.Value
	if row.As(&obj) != nil {
		return "", false, false
	}
	var name string
	if obj["name"].As(&name) != nil {
		return "", false, false
	}
	full := true
	for _, r := range rights {
		var b bool
		if obj[r].As(&b) != nil || !b {
			full = false
		}
	}
	return name, full, true
}

// keptValidator rejects a kept name with fewer than every right, which the
// server would ignore, so the plan would never settle.
type keptValidator struct{ kept []string }

func (v keptValidator) Description(context.Context) string {
	return fmt.Sprintf("%v always have every right", v.kept)
}

func (v keptValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v keptValidator) ValidateSet(_ context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	for _, e := range req.ConfigValue.Elements() {
		obj, ok := e.(types.Object)
		if !ok || obj.IsUnknown() {
			continue
		}
		attrs := obj.Attributes()
		name, ok := attrs["name"].(types.String)
		if !ok || name.IsUnknown() || !slices.Contains(v.kept, name.ValueString()) {
			continue
		}
		for _, r := range rights {
			if b, ok := attrs[r].(types.Bool); ok && !b.IsUnknown() && !b.ValueBool() {
				resp.Diagnostics.AddAttributeError(req.Path, "Built-in group always has full rights",
					fmt.Sprintf("The server always gives %s every right here and ignores anything less. Leave it out, or list it with can_view, can_modify and can_delete all true.", name.ValueString()))
				break
			}
		}
	}
}
