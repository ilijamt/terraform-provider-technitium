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
	// Columns are the row layout of a Table, in parameter order.
	Columns []Column

	// Key is the dotted path of the value in the read response. Without one
	// the attribute keeps its configured value.
	Key string

	Required bool
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

// ForceNew reports whether changing the attribute replaces the resource.
func (a Attr) ForceNew() bool { return a.Identity || !a.Update }

// Computed reports whether the server supplies the value when unset.
func (a Attr) Computed() bool { return !a.Required && a.Key != "" }

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
