package schema

import (
	"fmt"
	"strings"
)

// Registry is the runtime read-only view of a Definition.
// Constructed once at startup and safe for concurrent reads.
type Registry struct {
	resourceOrder       []string
	resources           map[string]*Resource
	fieldIndex          map[string]map[string]*Field  // resource -> fieldName -> *Field
	aliasIndex          map[string]map[string]string  // resource -> lowerInput -> canonical
	enumRoleIndex       map[string]*EnumRoleMapping   // role -> *EnumRoleMapping
	enumRolePrefixIndex map[string][]EnumRoleMapping  // prefix -> []EnumRoleMapping
}

// New constructs a Registry from a Definition.
// Panics if the definition contains obvious programmer errors (duplicate fields,
// enum role referencing a non-existent field) so mistakes surface at startup.
func New(def Definition) *Registry {
	r := &Registry{
		resources:           make(map[string]*Resource),
		fieldIndex:          make(map[string]map[string]*Field),
		aliasIndex:          make(map[string]map[string]string),
		enumRoleIndex:       make(map[string]*EnumRoleMapping),
		enumRolePrefixIndex: make(map[string][]EnumRoleMapping),
	}

	for i := range def.Resources {
		res := &def.Resources[i]
		r.resources[res.Name] = res
		r.resourceOrder = append(r.resourceOrder, res.Name)

		// Build field index
		r.fieldIndex[res.Name] = make(map[string]*Field)
		for j := range res.Fields {
			f := &res.Fields[j]
			r.fieldIndex[res.Name][f.Name] = f
		}

		// Build alias index (case-insensitive input)
		r.aliasIndex[res.Name] = make(map[string]string)
		for _, a := range res.Aliases {
			r.aliasIndex[res.Name][strings.ToLower(a.Input)] = a.Resolves
		}

		// Build enum role index and prefix index
		for j := range res.EnumRoles {
			m := &res.EnumRoles[j]
			r.enumRoleIndex[m.Role] = m
			// prefix = everything before the last dot segment
			if dotIdx := strings.LastIndex(m.Role, "."); dotIdx > 0 {
				prefix := m.Role[:dotIdx]
				r.enumRolePrefixIndex[prefix] = append(r.enumRolePrefixIndex[prefix], *m)
			}
		}
	}
	return r
}

// --- Resource-level lookups ---

// GetTableName returns the SQL table name for a resource.
func (r *Registry) GetTableName(resource string) (string, error) {
	if res, ok := r.resources[resource]; ok {
		return res.TableName, nil
	}
	return "", fmt.Errorf("unknown resourceType: %s", resource)
}

// GetDefaultSortField returns the default ORDER BY logical field for a resource.
func (r *Registry) GetDefaultSortField(resource string) (string, bool) {
	if res, ok := r.resources[resource]; ok {
		return res.DefaultSort, true
	}
	return "", false
}

// GetResource returns the full Resource definition.
func (r *Registry) GetResource(resource string) (*Resource, bool) {
	res, ok := r.resources[resource]
	return res, ok
}

// Resources returns all resource names in definition order.
func (r *Registry) Resources() []string {
	return r.resourceOrder
}

// --- Field-level lookups ---

// GetNativeColumn maps a canonical logical field name to the SQL column name.
func (r *Registry) GetNativeColumn(resource, field string) (string, bool) {
	if fields, ok := r.fieldIndex[resource]; ok {
		if f, ok := fields[field]; ok {
			return f.NativeColumn, true
		}
	}
	return "", false
}

// NativeColumnForField looks up a field name across all resources in order.
// Used when resource type is not known in advance (e.g. filter building).
func (r *Registry) NativeColumnForField(field string) (string, bool) {
	for _, rname := range r.resourceOrder {
		if n, ok := r.GetNativeColumn(rname, field); ok {
			return n, true
		}
	}
	return "", false
}

// NormalizeField resolves an aliased or raw field name to its canonical form.
// Returns (canonical, true) on success; (input, false) if not found.
func (r *Registry) NormalizeField(resource, input string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(input))
	if aliases, ok := r.aliasIndex[resource]; ok {
		if canonical, ok := aliases[key]; ok {
			return canonical, true
		}
	}
	// Already a valid canonical field?
	if fields, ok := r.fieldIndex[resource]; ok {
		if _, ok := fields[input]; ok {
			return input, true
		}
	}
	return input, false
}

// FilterableFields returns the set of logical field names usable in WHERE clauses.
func (r *Registry) FilterableFields(resource string) map[string]struct{} {
	out := make(map[string]struct{})
	if fields, ok := r.fieldIndex[resource]; ok {
		for name, f := range fields {
			if f.Filterable {
				out[name] = struct{}{}
			}
		}
	}
	return out
}

// SortableFields returns the set of logical field names usable in ORDER BY.
func (r *Registry) SortableFields(resource string) map[string]struct{} {
	out := make(map[string]struct{})
	if fields, ok := r.fieldIndex[resource]; ok {
		for name, f := range fields {
			if f.Sortable {
				out[name] = struct{}{}
			}
		}
	}
	return out
}

// IsFieldAllowed checks whether a field is allowed for a given resource + intent (V2).
func (r *Registry) IsFieldAllowed(resource, intent, field string) bool {
	if fields, ok := r.fieldIndex[resource]; ok {
		if f, ok := fields[field]; ok {
			for _, scope := range f.IntentScopes {
				if scope == intent || scope == "default" {
					return true
				}
			}
		}
	}
	return false
}

// GetEnumValues returns valid enum values for a field, or nil if not an enum.
func (r *Registry) GetEnumValues(resource, field string) []string {
	if fields, ok := r.fieldIndex[resource]; ok {
		if f, ok := fields[field]; ok {
			return f.EnumValues
		}
	}
	return nil
}

// --- Enum role lookups ---

// GetEnumRole returns the field + value mapping for a semantic role name.
func (r *Registry) GetEnumRole(role string) (EnumRoleMapping, bool) {
	if m, ok := r.enumRoleIndex[role]; ok {
		return *m, true
	}
	return EnumRoleMapping{}, false
}

// GetEnumRoleValue is a convenience that returns just the value string.
func (r *Registry) GetEnumRoleValue(role string) (string, bool) {
	if m, ok := r.enumRoleIndex[role]; ok {
		return m.Value, true
	}
	return "", false
}

// GetEnumRolesByPrefix returns all roles whose name begins with prefix + ".".
// Example: GetEnumRolesByPrefix("shipment.not_shipped_deny") returns the three deny roles.
func (r *Registry) GetEnumRolesByPrefix(prefix string) []EnumRoleMapping {
	return r.enumRolePrefixIndex[prefix]
}

// --- Identifier pattern lookups ---

// GetAllIdentifierPatterns returns every IdentifierPattern across all resources
// in resource-definition order.
func (r *Registry) GetAllIdentifierPatterns() []IdentifierPattern {
	var out []IdentifierPattern
	for _, rname := range r.resourceOrder {
		if res, ok := r.resources[rname]; ok {
			out = append(out, res.Identifiers...)
		}
	}
	return out
}

// GetIdentifierByType returns the primary resource and field for an identifier type string.
func (r *Registry) GetIdentifierByType(identifierType string) (resource string, field string, ok bool) {
	for _, rname := range r.resourceOrder {
		if res, ok2 := r.resources[rname]; ok2 {
			for _, ip := range res.Identifiers {
				if ip.IdentifierType == identifierType {
					return rname, ip.PrimaryField, true
				}
			}
		}
	}
	return "", "", false
}

// GetSecondaryLookups returns the secondary lookup specs for an identifier type.
func (r *Registry) GetSecondaryLookups(identifierType string) []SecondaryLookup {
	for _, rname := range r.resourceOrder {
		if res, ok := r.resources[rname]; ok {
			for _, ip := range res.Identifiers {
				if ip.IdentifierType == identifierType {
					return ip.SecondaryFields
				}
			}
		}
	}
	return nil
}

// --- V1 legacy support ---

// GetV1SearchableFields returns the V1 allowlist (native/alias field names) for a resource.
func (r *Registry) GetV1SearchableFields(resource string) map[string]struct{} {
	out := make(map[string]struct{})
	if res, ok := r.resources[resource]; ok {
		for _, f := range res.V1SearchableFields {
			out[f] = struct{}{}
		}
	}
	return out
}
