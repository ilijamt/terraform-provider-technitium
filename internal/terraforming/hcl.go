package terraforming

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
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

type variable struct {
	Name, Description string
}

type ref func(path []string, s string) (hclwrite.Tokens, bool)

func resourceBlock(body *hclwrite.Body, typ, label string, schema *tfprotov6.Schema, state map[string]tftypes.Value, refs ref, isJSON func(attr string) bool) []variable {
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
		if a.Sensitive && !literal(typ, a.Name) {
			// The server does not return some secrets, so a required one
			// gets a variable even without a value.
			if a.Required || !empty(v) || needsSecret(typ, a.Name, state) {
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
		if isJSON != nil && isJSON(a.Name) {
			var s string
			if v.As(&s) == nil {
				if t, ok := jsonencode(s); ok {
					b.SetAttributeRaw(a.Name, t)
					continue
				}
			}
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

func needsSecret(typ, attr string, state map[string]tftypes.Value) bool {
	return typ == "settings_sso" && attr == "client_secret" && !empty(state["client_id"]) ||
		typ == "settings_ldap" && attr == "bind_password" && !empty(state["bind_username"])
}

// literal secrets are written as they are: the server returns them, and a
// variable would lose the JSON layout that keeps the import plan empty.
func literal(typ, attr string) bool {
	return typ == "app_config" && attr == "config"
}

var secretValue = regexp.MustCompile(`(?i)(^|[;\s])(password|pwd)\s*=|^[a-z][a-z0-9+.-]*://[^/?#@\s]*:[^/?#@\s]*@|^(bearer|basic)\s+\S`)

// secretKeys returns only keys, so a warning never repeats a value.
func secretKeys(s string) []string {
	var v any
	if json.Unmarshal([]byte(s), &v) != nil {
		return nil
	}
	found := map[string]bool{}
	var walk func(key string, v any)
	walk = func(key string, v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, e := range v {
				l := strings.ToLower(k)
				for _, w := range []string{"password", "passwd", "secret", "token", "apikey", "api_key", "credential", "connectionstring", "authorization"} {
					if strings.Contains(l, w) {
						found[k] = true
					}
				}
				if strings.HasSuffix(l, "key") {
					found[k] = true
				}
				walk(k, e)
			}
		case []any:
			for _, e := range v {
				walk(key, e)
			}
		case string:
			if key != "" && secretValue.MatchString(v) {
				found[key] = true
			}
		}
	}
	walk("", v)
	if len(found) == 0 {
		return nil
	}
	return sortedKeys(found)
}

// owned attributes are written even when empty, so an entry added outside
// Terraform shows as a change.
func owned(typ, attr string) bool {
	return permissions(typ) && (attr == "user_permissions" || attr == "group_permissions") ||
		typ == "settings_tsig" && attr == "tsig_keys"
}

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

type writer struct {
	sensitive func(path []string) bool
	secret    func(path []string) hclwrite.Tokens
	refs      ref
}

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
			out = append(out, hclwrite.ObjectAttrTokens{Name: objectKey(k), Value: w.tokens(fields[k], append(slices.Clone(path), k))})
		}
		return hclwrite.TokensForObject(out)
	}
}

// Empty strings and collections are left out: the server reads them back
// the same either way.
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

func dataBlock(body *hclwrite.Body, typ, label string, identity map[string]string) {
	b := body.AppendNewBlock("data", []string{"technitium_" + typ, label}).Body()
	for _, k := range sortedKeys(identity) {
		b.SetAttributeValue(k, cty.StringVal(identity[k]))
	}
}

// The ID stays a string literal even when it is JSON: Terraform 1.5 allows
// no function calls in an import block.
func importBlock(body *hclwrite.Body, typ, label, id string) {
	b := body.AppendNewBlock("import", nil).Body()
	b.SetAttributeTraversal("to", hcl.Traversal{hcl.TraverseRoot{Name: "technitium_" + typ}, hcl.TraverseAttr{Name: label}})
	b.SetAttributeValue("id", cty.StringVal(id))
}

func jsonencode(s string) (hclwrite.Tokens, bool) {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil || d.More() {
		return nil, false
	}
	switch v.(type) {
	case map[string]any, []any:
		return hclwrite.TokensForFunctionCall("jsonencode", jsonTokens(v)), true
	}
	return nil, false
}

func jsonTokens(v any) hclwrite.Tokens {
	switch v := v.(type) {
	case map[string]any:
		var out []hclwrite.ObjectAttrTokens
		for _, k := range sortedKeys(v) {
			out = append(out, hclwrite.ObjectAttrTokens{Name: objectKey(k), Value: jsonTokens(v[k])})
		}
		return hclwrite.TokensForObject(out)
	case []any:
		out := make([]hclwrite.Tokens, len(v))
		for i, e := range v {
			out[i] = jsonTokens(e)
		}
		return hclwrite.TokensForTuple(out)
	case string:
		return hclwrite.TokensForValue(cty.StringVal(v))
	case json.Number:
		if n, err := cty.ParseNumberVal(v.String()); err == nil {
			return hclwrite.TokensForValue(n)
		}
		return hclwrite.TokensForValue(cty.StringVal(v.String()))
	case bool:
		return hclwrite.TokensForValue(cty.BoolVal(v))
	}
	return hclwrite.TokensForValue(cty.NullVal(cty.DynamicPseudoType))
}

// objectKey quotes a key that is not an identifier, or is a keyword: a
// leading for turns the object into a for expression.
func objectKey(k string) hclwrite.Tokens {
	switch k {
	case "for", "in", "if", "null", "true", "false":
	default:
		if hclsyntax.ValidIdentifier(k) {
			return hclwrite.TokensForIdentifier(k)
		}
	}
	return hclwrite.TokensForValue(cty.StringVal(k))
}

func variableBlock(body *hclwrite.Body, v variable) {
	b := body.AppendNewBlock("variable", []string{v.Name}).Body()
	b.SetAttributeValue("description", cty.StringVal(v.Description))
	b.SetAttributeTraversal("type", hcl.Traversal{hcl.TraverseRoot{Name: "string"}})
	b.SetAttributeValue("sensitive", cty.True)
}

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
