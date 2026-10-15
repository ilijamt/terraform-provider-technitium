// Package framework implements every Technitium resource from a Spec.
package framework

// Spec describes one resource. The specs are written by hand in package
// technitium.
type Spec struct {
	Name        string
	Description string

	Create string
	Read   string
	Update string
	Delete string

	// List names the array in the read response holding the objects; the
	// one matching the identity attributes and Fixed is used.
	List string
	// Fixed parameters are sent on every call.
	Fixed map[string]string
	// CreateFixed parameters are sent on the create call only.
	CreateFixed map[string]string
	// Adopt is for an object that always exists and whose update call needs
	// every value: create reads it and updates it with the plan.
	Adopt bool
	// Created returns values, by attribute name, that only the create call's
	// response holds, such as a token shown once. They go into state, and the
	// read after create uses them to find the object.
	Created func(resp map[string]any) map[string]any

	Attrs []Attr
}

type Type string

const (
	String Type = "string"
	Bool   Type = "bool"
	Int64  Type = "int"
	List   Type = "list"
	// Map is string keys to string values. The API takes "key|value|..."
	// and returns an object.
	Map Type = "map"
	// Table is a set of rows with the attribute's Columns. The API takes
	// every cell of every row joined by "|" and returns the rows as objects,
	// in an order of its own.
	Table Type = "table"
)

// Toggle is a Bool attribute the API changes with a call of its own for each
// value, given the identity parameters, instead of a parameter.
type Toggle struct {
	On, Off string
}

// Source is where an attribute is read when the read call does not return
// it: the item of List in the response of Path, matched on the identity like
// the read call's List.
type Source struct {
	Path, List string
}

// Column is one value in each row of a Table attribute. A List column is
// sent with its values joined by ",".
type Column struct {
	Name string
	// Key is the row object's key in the read response.
	Key         string
	Type        Type
	Sensitive   bool
	Description string
	// Pattern is a regular expression a String value must match.
	Pattern string
}

type Attr struct {
	Name  string
	Param string
	// CreateParam names the parameter in the create call when it differs
	// from Param.
	CreateParam string
	Type        Type
	Description string
	Enum        []string
	// Pattern is a regular expression a String value must match, for values
	// the server would store in another spelling.
	Pattern string
	// Empty is the value a String has when the server returns "", for a
	// value it takes but stores as empty, like "." for a record's own name.
	Empty string
	// JSON marks a String holding JSON. Values that differ only in spacing
	// and key order count as equal, so jsonencode(...) matches what the
	// server keeps.
	JSON bool
	// Separator joins the values of a List when it is not ",".
	Separator string
	// Columns are the row layout of a Table, in parameter order.
	Columns []Column
	// Kept are the names, in a permissions Table, the server always keeps
	// with every right. They may be left out of the configuration and must
	// not be given less.
	Kept []string
	// Toggle sets a Bool through its own calls; Source reads a value the
	// read call lacks.
	Toggle *Toggle
	Source *Source

	// Key is the dotted path of the value in the read response. Without one
	// the attribute keeps its configured value.
	Key string

	Required bool
	// DataOnly attributes are only on the data source, for values that mean
	// nothing for what the resource manages.
	DataOnly bool
	// ReadOnly attributes come from the server and are never configured.
	// The update call still gets the last value read.
	ReadOnly  bool
	Sensitive bool
	// Identity attributes locate the object and are sent on read, update and delete.
	Identity bool
	// Create and Update say which calls accept the attribute. One accepted
	// only by Update is applied right after create.
	Create bool
	Update bool
}

// ForceNew reports whether changing the attribute replaces the resource. A
// read-only value changes on the server's account, never by replacing.
func (a Attr) ForceNew() bool { return !a.ReadOnly && (a.Identity || !a.Update) }

// Computed reports whether the server supplies the value when unset, or the
// value is only ever read.
func (a Attr) Computed() bool { return !a.Required && (a.Key != "" || a.ReadOnly) }

// Singleton resources have no delete call: they only update server state.
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
