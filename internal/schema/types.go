package schema

import "regexp"

// Field describes a single logical field in a resource.
type Field struct {
	Name         string   // Canonical logical name, e.g. "order.state"
	NativeColumn string   // SQL column name in the backing store, e.g. "order_state"
	Type         string   // "string" | "int" | "float" | "timestamp" | "enum"
	Description  string
	EnumValues   []string // Non-nil only when Type == "enum"
	Operators    []string // Allowed filter operators for this field
	IntentScopes []string // Intent categories that may filter on this field
	Example      string
	Sortable     bool // Whether this field can be used in ORDER BY
	Filterable   bool // Whether this field can be used in WHERE clauses
}

// Alias maps an input field name (possibly legacy or shorthand) to
// the canonical field name for a given resource.
type Alias struct {
	Input    string // e.g. "order_state", "status"
	Resolves string // e.g. "order.state"
}

// IdentifierPattern describes a regex-based identifier detection pattern.
type IdentifierPattern struct {
	Name            string         // e.g. "ord_default"
	IdentifierType  string         // matches an identifier.IdentifierType constant value
	Regex           *regexp.Regexp // compiled detection regex
	ResourceType    string         // which resource this identifier belongs to
	PrimaryField    string         // the direct match field, e.g. "order.number"
	SecondaryFields []SecondaryLookup
}

// SecondaryLookup encodes a derived lookup produced by a detected identifier.
// Example: a CUST- number also triggers an order.customer_id lookup.
type SecondaryLookup struct {
	ResourceType string
	Field        string
	// Transformer is applied to the normalized identifier value before use.
	// nil means use the normalized value directly.
	Transformer func(normalizedValue string) string
	Confidence  float64
}

// EnumRoleMapping binds a semantic role name to a concrete field + value.
// Role names use dot notation: "<scope>.<semantic_meaning>"
// e.g. "order.open_state", "shipment.not_shipped_deny.shipped"
type EnumRoleMapping struct {
	Role  string // e.g. "order.open_state"
	Field string // e.g. "order.state"
	Value string // e.g. "Open"
}

// Resource is the complete definition of one resource type.
type Resource struct {
	Name        string
	TableName   string // SQL table name, e.g. "orders_docs"
	DefaultSort string // default ORDER BY logical field
	Fields      []Field
	Aliases     []Alias
	Identifiers []IdentifierPattern
	EnumRoles   []EnumRoleMapping
	// V1SearchableFields is the legacy set of field alias names allowed under
	// contract v1. These use native/alias names, not canonical logical names.
	V1SearchableFields []string
}

// Definition is the top-level schema object passed to New().
type Definition struct {
	Resources []Resource
}
