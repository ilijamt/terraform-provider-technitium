package framework

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
)

func TestDropKept(t *testing.T) {
	a := Attr{Name: "group_permissions", Type: Table, Kept: []string{"Administrators"}, Columns: []Column{
		{Name: "name", Type: String}, {Name: "can_view", Type: Bool}, {Name: "can_modify", Type: Bool}, {Name: "can_delete", Type: Bool},
	}}
	row := func(name string, all bool) tftypes.Value {
		return tftypes.NewValue(a.row(), map[string]tftypes.Value{
			"name": str(name), "can_view": boolean(true), "can_modify": boolean(all), "can_delete": boolean(all),
		})
	}
	rows := func(r ...tftypes.Value) tftypes.Value { return tftypes.NewValue(a.tf(), r) }
	server := rows(row("Administrators", true), row("Ops", false))

	assert.True(t, rows(row("Ops", false)).Equal(dropKept(a, server, rows(row("Ops", false)))), "left out of the config: dropped")
	assert.True(t, server.Equal(dropKept(a, server, rows(row("Administrators", true), row("Ops", false)))), "listed: kept")
	assert.True(t, rows(row("Ops", false)).Equal(dropKept(a, server, tftypes.NewValue(a.tf(), nil))), "imported: dropped")
}

func TestKeptValidator(t *testing.T) {
	rowType := map[string]attr.Type{"name": types.StringType, "can_view": types.BoolType, "can_modify": types.BoolType, "can_delete": types.BoolType}
	row := func(name string, all bool) attr.Value {
		return types.ObjectValueMust(rowType, map[string]attr.Value{
			"name": types.StringValue(name), "can_view": types.BoolValue(true), "can_modify": types.BoolValue(all), "can_delete": types.BoolValue(all),
		})
	}
	check := func(rows ...attr.Value) bool {
		resp := &validator.SetResponse{}
		keptValidator{[]string{"Administrators"}}.ValidateSet(context.Background(), validator.SetRequest{
			ConfigValue: types.SetValueMust(types.ObjectType{AttrTypes: rowType}, rows),
		}, resp)
		return resp.Diagnostics.HasError()
	}
	assert.False(t, check(row("Administrators", true), row("Ops", false)))
	assert.True(t, check(row("Administrators", false)), "less than full rights for a kept group")
}

func TestMissing(t *testing.T) {
	a := Attr{Name: "group_permissions", Type: Table, Kept: []string{"Administrators"}, Refers: "group", Columns: []Column{
		{Name: "name", Type: String}, {Name: "can_view", Type: Bool}, {Name: "can_modify", Type: Bool}, {Name: "can_delete", Type: Bool},
	}}
	r := &Resource{spec: Spec{Name: "permissions_logs", Attrs: []Attr{a}}}
	row := func(name string) tftypes.Value {
		return tftypes.NewValue(a.row(), map[string]tftypes.Value{
			"name": str(name), "can_view": boolean(true), "can_modify": boolean(true), "can_delete": boolean(true),
		})
	}
	rows := func(r ...tftypes.Value) map[string]tftypes.Value {
		return map[string]tftypes.Value{a.Name: tftypes.NewValue(a.tf(), r)}
	}

	assert.NoError(t, r.missing(rows(row("Administrators"), row("Ops")), rows(row("Ops"))), "kept names are never missing")
	assert.EqualError(t, r.missing(rows(row("Ops"), row("Nope")), rows(row("Ops"))),
		`permissions_logs: group "Nope" does not exist; create it first or refer to its resource so it's created before this`)
	assert.NoError(t, r.missing(map[string]tftypes.Value{a.Name: tftypes.NewValue(a.tf(), nil)}, rows(row("Ops"))), "unset")
}
