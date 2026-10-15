package terraforming

import (
	"fmt"
	"math/big"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/zclconf/go-cty/cty"
)

// variable is a sensitive value the generated configuration takes from a
// variable instead of holding it.
type variable struct {
	Name, Description string
}

// ref returns a reference to use instead of the string at path, which starts
// with the attribute name, or false to write the string.
type ref func(path []string, s string) (hclwrite.Tokens, bool)

// resourceBlock writes the configuration of one imported object. Sensitive
// values become references to new variables, which it returns; strings
// refs knows become references.
func resourceBlock(body *hclwrite.Body, typ, label string, schema *tfprotov6.Schema, state map[string]tftypes.Value, refs ref) []variable {
	b := body.AppendNewBlock("resource", []string{"technitium_" + typ, label}).Body()
	var vars []variable
	secret := func(name, what string) hclwrite.Tokens {
		v := variable{Name: name, Description: fmt.Sprintf("%s of technitium_%s.%s.", what, typ, label)}
		vars = append(vars, v)
		return hclwrite.TokensForTraversal(hcl.Traversal{hcl.TraverseRoot{Name: "var"}, hcl.TraverseAttr{Name: v.Name}})
	}

	attrs := slices.Clone(schema.Block.Attributes)
	sort.SliceStable(attrs, func(i, j int) bool { return attrs[i].Required && !attrs[j].Required })
	for _, a := range attrs {
		v := state[a.Name]
		readOnly := a.Computed && !a.Optional && !a.Required
		if a.Name == "id" || readOnly {
			continue
		}
		if a.Sensitive {
			// The server does not return some secrets, so a required one
			// gets a variable even without a value.
			if a.Required || !empty(v) {
				b.SetAttributeRaw(a.Name, secret(typ+"_"+label+"_"+a.Name, "The "+a.Name))
			}
			continue
		}
		if !a.Required && !owned(typ, a.Name) && empty(v) || v.IsNull() {
			continue
		}
		w := writer{refs: refs, secret: func(path []string) hclwrite.Tokens {
			return secret(typ+"_"+label+"_"+strings.Join(path, "_"), "The "+strings.Join(path, " "))
		}}
		if a.NestedType != nil {
			w.sensitive = func(path []string) bool { return nestedSensitive(a.NestedType, path) }
		}
		b.SetAttributeRaw(a.Name, w.tokens(v, []string{a.Name}))
	}
	// The users existed before Terraform, and deleting one, such as admin,
	// locks people out.
	if typ == "user" {
		b.AppendNewline()
		b.AppendNewBlock("lifecycle", nil).Body().SetAttributeValue("prevent_destroy", cty.True)
	}
	return vars
}

// owned reports whether an attribute is written even when empty, so that
// Terraform owns the list: an entry added outside it shows as a change.
func owned(typ, attr string) bool {
	return strings.HasPrefix(typ, "permissions_") && (attr == "user_permissions" || attr == "group_permissions") ||
		typ == "settings_tsig" && attr == "tsig_keys"
}

// nestedSensitive reports whether the nested attribute at path (attribute,
// element index, nested attribute) is sensitive.
func nestedSensitive(obj *tfprotov6.SchemaObject, path []string) bool {
	if len(path) < 3 {
		return false
	}
	for _, a := range obj.Attributes {
		if a.Name == path[len(path)-1] {
			return a.Sensitive
		}
	}
	return false
}

// writer renders values as HCL. A value at a sensitive path is replaced by
// secret(path), and a string refs knows by a reference.
type writer struct {
	sensitive func(path []string) bool
	secret    func(path []string) hclwrite.Tokens
	refs      ref
}

// tokens renders a value as HCL. Collections become tuples, maps and objects
// become objects.
func (w writer) tokens(v tftypes.Value, path []string) hclwrite.Tokens {
	if w.sensitive != nil && w.sensitive(path) {
		return w.secret(path)
	}
	switch {
	case v.IsNull():
		return hclwrite.TokensForValue(cty.NullVal(cty.DynamicPseudoType))
	case v.Type().Is(tftypes.String):
		var s string
		_ = v.As(&s)
		if w.refs != nil {
			if t, ok := w.refs(path, s); ok {
				return t
			}
		}
		return hclwrite.TokensForValue(cty.StringVal(s))
	case v.Type().Is(tftypes.Number):
		var f big.Float
		_ = v.As(&f)
		return hclwrite.TokensForValue(cty.NumberVal(&f))
	case v.Type().Is(tftypes.Bool):
		var b bool
		_ = v.As(&b)
		return hclwrite.TokensForValue(cty.BoolVal(b))
	case v.Type().Is(tftypes.List{}), v.Type().Is(tftypes.Set{}), v.Type().Is(tftypes.Tuple{}):
		var elems []tftypes.Value
		_ = v.As(&elems)
		out := make([]hclwrite.Tokens, len(elems))
		for i, e := range elems {
			out[i] = w.tokens(e, append(slices.Clone(path), fmt.Sprint(i)))
		}
		return hclwrite.TokensForTuple(out)
	default:
		var fields map[string]tftypes.Value
		_ = v.As(&fields)
		var out []hclwrite.ObjectAttrTokens
		for _, k := range sortedKeys(fields) {
			if fields[k].IsNull() {
				continue
			}
			name := hclwrite.TokensForValue(cty.StringVal(k))
			if hclsyntax.ValidIdentifier(k) {
				name = hclwrite.TokensForIdentifier(k)
			}
			out = append(out, hclwrite.ObjectAttrTokens{Name: name, Value: w.tokens(fields[k], append(slices.Clone(path), k))})
		}
		return hclwrite.TokensForObject(out)
	}
}

// empty reports values left out of the configuration: unset ones, and empty
// strings and collections, which the server reads back the same either way.
func empty(v tftypes.Value) bool {
	if !v.IsKnown() || v.IsNull() {
		return true
	}
	if v.Type().Is(tftypes.String) {
		var s string
		_ = v.As(&s)
		return s == ""
	}
	if v.Type().Is(tftypes.List{}) || v.Type().Is(tftypes.Set{}) {
		var elems []tftypes.Value
		_ = v.As(&elems)
		return len(elems) == 0
	}
	if v.Type().Is(tftypes.Map{}) {
		var elems map[string]tftypes.Value
		_ = v.As(&elems)
		return len(elems) == 0
	}
	return false
}

// dataBlock writes a data source for an object Terraform cannot delete,
// found by its identity attributes.
func dataBlock(body *hclwrite.Body, typ, label string, identity map[string]string) {
	b := body.AppendNewBlock("data", []string{"technitium_" + typ, label}).Body()
	for _, k := range sortedKeys(identity) {
		b.SetAttributeValue(k, cty.StringVal(identity[k]))
	}
}

func importBlock(body *hclwrite.Body, typ, label, id string) {
	b := body.AppendNewBlock("import", nil).Body()
	b.SetAttributeTraversal("to", hcl.Traversal{hcl.TraverseRoot{Name: "technitium_" + typ}, hcl.TraverseAttr{Name: label}})
	b.SetAttributeValue("id", cty.StringVal(id))
}

func variableBlock(body *hclwrite.Body, v variable) {
	b := body.AppendNewBlock("variable", []string{v.Name}).Body()
	b.SetAttributeValue("description", cty.StringVal(v.Description))
	b.SetAttributeTraversal("type", hcl.Traversal{hcl.TraverseRoot{Name: "string"}})
	b.SetAttributeValue("sensitive", cty.True)
}

// labels turns names into unique resource labels.
type labels map[string]bool

func (l labels) next(name string) string {
	label := sanitize(name)
	unique := label
	for i := 2; l[unique]; i++ {
		unique = fmt.Sprintf("%s_%d", label, i)
	}
	l[unique] = true
	return unique
}

// sanitize turns a name into a label: lower case letters, digits and
// underscores, "*" spelled out, never starting with a digit.
func sanitize(name string) string {
	var b strings.Builder
	for _, r := range strings.ReplaceAll(strings.ToLower(name), "*", "wildcard") {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	label := strings.Trim(b.String(), "_")
	for strings.Contains(label, "__") {
		label = strings.ReplaceAll(label, "__", "_")
	}
	if label == "" || label[0] >= '0' && label[0] <= '9' {
		label = "r_" + label
	}
	return label
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
