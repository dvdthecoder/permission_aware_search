package semantic

import (
	"strings"

	"permission_aware_search/internal/contracts"
	"permission_aware_search/internal/store"
)

// hasAuthoritativeSupportFilter returns true when parsed filters already
// express explicit support constraints and should remain authoritative.
// In that case, semantic/vector candidates must not be applied as a hard id IN.
func hasAuthoritativeSupportFilter(resourceType string, filters []store.Filter) bool {
	for _, f := range filters {
		field := contracts.NormalizeField(resourceType, f.Field)
		if field == "" {
			field = strings.TrimSpace(strings.ToLower(f.Field))
		}
		if field == "" {
			continue
		}
		if field == "id" && strings.EqualFold(strings.TrimSpace(f.Op), "in") {
			continue
		}
		// Temporal-only scoping should not disable semantic narrowing.
		if field == "order.created_at" || field == "order.completed_at" ||
			field == "customer.created_at" || field == "customer.last_modified_at" {
			continue
		}
		return true
	}
	return false
}
