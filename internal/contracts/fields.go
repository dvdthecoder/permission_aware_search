package contracts

import (
	"strings"

	"permission_aware_search/internal/schema"
)

// defaultRegistry is set once at startup via SetRegistry.
// Package-level functions delegate to it for field normalization.
var defaultRegistry *schema.Registry

// SetRegistry wires the schema registry into this package.
// Must be called before serving requests.
func SetRegistry(r *schema.Registry) {
	defaultRegistry = r
}

// DefaultRegistry returns the package-level schema registry (may be nil before startup wiring).
func DefaultRegistry() *schema.Registry {
	return defaultRegistry
}

// NormalizeField resolves an aliased or raw field name to its canonical form.
// Delegates to the schema registry; falls back to the input if no registry
// is set (e.g. in isolated unit tests that don't wire the full stack).
func NormalizeField(resourceType, field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	if defaultRegistry != nil {
		if canonical, ok := defaultRegistry.NormalizeField(resourceType, field); ok {
			return canonical
		}
	}
	return field
}
