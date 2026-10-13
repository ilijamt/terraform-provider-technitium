package framework

import (
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func (t Type) tf() tftypes.Type {
	switch t {
	case Bool:
		return tftypes.Bool
	case Int64:
		return tftypes.Number
	case List:
		return tftypes.List{ElementType: tftypes.String}
	case Map:
		return tftypes.Map{ElementType: tftypes.String}
	}
	return tftypes.String
}

func set(v tftypes.Value) bool { return v.IsKnown() && !v.IsNull() }

// param renders a known, non-null value as an API parameter. Lists are comma
// separated; an empty list is "", which the API treats as clearing it.
func param(v tftypes.Value, t Type) (string, error) {
	switch t {
	case Bool:
		var b bool
		err := v.As(&b)
		return strconv.FormatBool(b), err
	case Int64:
		var f big.Float
		err := v.As(&f)
		i, _ := f.Int64()
		return strconv.FormatInt(i, 10), err
	case List:
		var elems []tftypes.Value
		if err := v.As(&elems); err != nil {
			return "", err
		}
		out := make([]string, len(elems))
		for i, e := range elems {
			if err := e.As(&out[i]); err != nil {
				return "", err
			}
		}
		return strings.Join(out, ","), nil
	case Map:
		var elems map[string]tftypes.Value
		if err := v.As(&elems); err != nil {
			return "", err
		}
		var cells []string
		for _, k := range slices.Sorted(maps.Keys(elems)) {
			var s string
			if err := elems[k].As(&s); err != nil {
				return "", err
			}
			cells = append(cells, k, s)
		}
		return strings.Join(cells, "|"), nil
	}
	var s string
	err := v.As(&s)
	return s, err
}

// fromJSON converts a response value, or an imported string. An error means the
// value has a shape the attribute cannot represent, so the caller keeps its own.
func fromJSON(raw any, t Type) (tftypes.Value, error) {
	if raw == nil {
		return tftypes.NewValue(t.tf(), nil), nil
	}
	switch t {
	case Bool:
		switch v := raw.(type) {
		case bool:
			return tftypes.NewValue(tftypes.Bool, v), nil
		case string:
			b, err := strconv.ParseBool(v)
			return tftypes.NewValue(tftypes.Bool, b), err
		}
	case Int64:
		var s string
		switch v := raw.(type) {
		case json.Number:
			s = v.String()
		case string:
			s = v
		default:
			return tftypes.Value{}, fmt.Errorf("not a number: %T", raw)
		}
		i, err := strconv.ParseInt(s, 10, 64)
		return tftypes.NewValue(tftypes.Number, new(big.Float).SetInt64(i)), err
	case List:
		var items []string
		switch v := raw.(type) {
		case []any:
			for _, e := range v {
				s, err := scalar(e)
				if err != nil {
					return tftypes.Value{}, err
				}
				items = append(items, s)
			}
		case string:
			if v != "" {
				items = strings.Split(v, ",")
			}
		default:
			return tftypes.Value{}, fmt.Errorf("not a list: %T", raw)
		}
		elems := make([]tftypes.Value, len(items))
		for i, s := range items {
			elems[i] = tftypes.NewValue(tftypes.String, strings.TrimSpace(s))
		}
		return tftypes.NewValue(t.tf(), elems), nil
	case Map:
		elems := map[string]tftypes.Value{}
		switch v := raw.(type) {
		case map[string]any:
			for k, e := range v {
				s, err := scalar(e)
				if err != nil {
					return tftypes.Value{}, err
				}
				elems[k] = tftypes.NewValue(tftypes.String, s)
			}
		case string:
			// An import ID part, in the "key|value|..." form the API takes.
			cells := strings.Split(v, "|")
			if v == "" || len(cells)%2 != 0 {
				return tftypes.Value{}, fmt.Errorf("want key|value pairs, got %q", v)
			}
			for i := 0; i < len(cells); i += 2 {
				elems[cells[i]] = tftypes.NewValue(tftypes.String, cells[i+1])
			}
		default:
			return tftypes.Value{}, fmt.Errorf("not a map: %T", raw)
		}
		return tftypes.NewValue(t.tf(), elems), nil
	default:
		s, err := scalar(raw)
		return tftypes.NewValue(tftypes.String, s), err
	}
	return tftypes.Value{}, fmt.Errorf("unexpected %T for %s", raw, t)
}

func scalar(raw any) (string, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	case bool:
		return strconv.FormatBool(v), nil
	}
	return "", fmt.Errorf("not a scalar: %T", raw)
}

// lookup follows a dotted key through nested objects.
func lookup(obj map[string]any, key string) (any, bool) {
	var cur any = obj
	for _, k := range strings.Split(key, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[k]; !ok {
			return nil, false
		}
	}
	return cur, true
}

// sameName compares DNS names and other identity values loosely.
func sameName(a, b string) bool {
	return strings.EqualFold(strings.TrimSuffix(a, "."), strings.TrimSuffix(b, "."))
}

// tf is the Terraform type of the attribute.
func (a Attr) tf() tftypes.Type {
	if a.Type == Table {
		return tftypes.Set{ElementType: a.row()}
	}
	return a.Type.tf()
}

func (a Attr) row() tftypes.Object {
	types := map[string]tftypes.Type{}
	for _, c := range a.Columns {
		types[c.Name] = c.Type.tf()
	}
	return tftypes.Object{AttributeTypes: types}
}

// param renders a known, non-null value of the attribute as an API
// parameter. A table is every cell of every row, in column order, joined by
// "|"; no rows is "", which the API treats as clearing it.
func (a Attr) param(v tftypes.Value) (string, error) {
	if a.Type != Table {
		return param(v, a.Type)
	}
	var rows []tftypes.Value
	if err := v.As(&rows); err != nil {
		return "", err
	}
	var cells []string
	for _, row := range rows {
		var obj map[string]tftypes.Value
		if err := row.As(&obj); err != nil {
			return "", err
		}
		for _, c := range a.Columns {
			s, err := param(obj[c.Name], c.Type)
			if err != nil {
				return "", fmt.Errorf("%s: %w", c.Name, err)
			}
			cells = append(cells, s)
		}
	}
	return strings.Join(cells, "|"), nil
}

// fromJSON converts a response value of the attribute, see fromJSON.
func (a Attr) fromJSON(raw any) (tftypes.Value, error) {
	if a.Type != Table {
		return fromJSON(raw, a.Type)
	}
	if raw == nil {
		return tftypes.NewValue(a.tf(), nil), nil
	}
	items, ok := raw.([]any)
	if !ok {
		return tftypes.Value{}, fmt.Errorf("not a list: %T", raw)
	}
	rows := make([]tftypes.Value, len(items))
	for i, it := range items {
		item, ok := it.(map[string]any)
		if !ok {
			return tftypes.Value{}, fmt.Errorf("not an object: %T", it)
		}
		obj := map[string]tftypes.Value{}
		for _, c := range a.Columns {
			v, err := fromJSON(item[c.Key], c.Type)
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("%s: %w", c.Name, err)
			}
			obj[c.Name] = v
		}
		rows[i] = tftypes.NewValue(a.row(), obj)
	}
	return tftypes.NewValue(a.tf(), rows), nil
}
