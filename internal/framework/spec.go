package framework

import (
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type Spec struct {
	Name        string
	Description string

	Create string
	Read   string
	Update string
	Delete string

	List        string
	Fixed       map[string]string
	CreateFixed map[string]string
	// Adopt is for an object that always exists and whose update call needs
	// every value: create reads it and updates it with the plan.
	Adopt bool
	// Created returns values only the create call's response holds, such as
	// a token shown once. The read after create uses them to find the object.
	Created func(resp map[string]any) map[string]any
	Check   func(prior, plan map[string]tftypes.Value) error
	// UpdateOnce is for an update call that finds the object by its old
	// values, so resending it after a dropped connection fails.
	UpdateOnce bool
	// Shared names an attribute every List item shares, such as the TTL of
	// the records at one name and type, which the server sets on all of them
	// when one changes. An unset value takes theirs, and another value is
	// refused unless the object already had theirs and moves them all.
	// Creating an object that already exists is refused too, since the
	// server would take it over with the new value.
	Shared string

	Attrs []Attr
}

type Type string

const (
	String Type = "string"
	Bool   Type = "bool"
	Int64  Type = "int"
	List   Type = "list"
	// Map is sent as "key|value|..." and returned as an object.
	Map Type = "map"
	// Table is a set of rows, sent as every cell joined by "|" and returned
	// as objects in an order of the server's own.
	Table Type = "table"
)

// Toggle is a Bool the API sets with a call per value instead of a parameter.
type Toggle struct {
	On, Off string
}

// Convert changes a String in place with a call to Path taking the identity
// and the new value, for the changes From lists by old value; any other
// change replaces the resource. The object comes back with the defaults of
// its new kind, so the values left unconfigured are read again.
type Convert struct {
	Path string
	From map[string][]string
}

func (c *Convert) allows(from, to string) bool {
	return c != nil && slices.Contains(c.From[from], to)
}

// Source reads an attribute the read call lacks from the List item of
// another call's response, matched on the identity.
type Source struct {
	Path, List string
}

type Column struct {
	Name        string
	Key         string
	Type        Type
	Sensitive   bool
	Description string
	Pattern     string
}

type Attr struct {
	Name        string
	Param       string
	CreateParam string
	Type        Type
	Description string
	Enum        []string
	// Pattern guards values the server would store in another spelling.
	Pattern    string
	Validators []validator.String
	Compare    Compare
	// Empty is what a String reads as when the server returns "", for a
	// value it takes but stores as empty, like "." for a record's own name.
	Empty string
	// JSON values are compared by content and read back as jsonencode
	// writes them.
	JSON      bool
	Separator string
	// Auto names the Map keys the server fills in itself while the named Bool
	// attribute is on. They are ignored when matching and reading, and calls
	// that locate the object send the value as the server stores it.
	Auto map[string]string
	// NewParam is the update call's parameter for the planned value of an
	// identity attribute sent as the server stores it.
	NewParam string
	// Columns are in the order the API takes the cells.
	Columns []Column
	// Kept are names in a permissions Table the server always keeps with
	// every right; they may be left out of the configuration.
	Kept []string
	// Refers is the kind of object the first column of a Table names, such
	// as "user": the server drops a name that does not exist without an
	// error, and the provider reports it.
	Refers  string
	Toggle  *Toggle
	Convert *Convert
	Source  *Source
	// Resets names the attributes the server sets anew when this value
	// changes; they are planned unknown unless configured.
	Resets []string

	// Default is the bool, int or string an unconfigured value takes, and
	// what a response without Key, or with a null, reads as. The create call
	// leaves it out.
	Default any

	// Key is the dotted path of the value in the read response. Without one
	// the attribute keeps its configured value.
	Key string

	Required bool
	DataOnly bool
	// ReadOnly values are never configured, but the update call still gets
	// the last value read.
	ReadOnly  bool
	Sensitive bool
	Identity  bool
	// An attribute only Update accepts is applied right after create.
	Create bool
	Update bool
	// OnChange sends the value in an update only when it differs from the
	// prior state, for values the server never returns and an update replaces.
	OnChange bool
	// Restarts is for a value whose change makes the server restart its web
	// service a moment after the call returns.
	Restarts bool
}

// A read-only value changes on the server's account, never by replacing.
func (a Attr) ForceNew() bool { return !a.ReadOnly && (a.Identity || !a.Update) }

func (a Attr) Computed() bool { return !a.Required && (a.Key != "" || a.ReadOnly) }

func (s Spec) Singleton() bool { return s.Delete == "" }

func (s Spec) identity() []Attr {
	var out []Attr
	for _, a := range s.Attrs {
		if a.Identity {
			out = append(out, a)
		}
	}
	return out
}
