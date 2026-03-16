package contracts

import "fmt"

const (
	ContractVersionV1 = "v1"
	ContractVersionV2 = "v2"

	IntentDefault        = "default"
	IntentWISMO          = "wismo"
	IntentCRMProfile     = "crm_profile"
	IntentReturnsRefunds = "returns_refunds"
	IntentUnsupported    = "unsupported_domain"
)

// ValidateField checks whether a field is allowed for the given resource,
// contract version, and intent.  All allowlist data comes from the schema
// registry; no hardcoded maps remain here.
func ValidateField(resourceType, version, intent, field string) error {
	if intent == "" {
		intent = IntentDefault
	}

	switch version {
	case ContractVersionV1:
		if defaultRegistry == nil {
			return fmt.Errorf("schema registry not initialised")
		}
		allowed := defaultRegistry.GetV1SearchableFields(resourceType)
		if len(allowed) == 0 {
			return fmt.Errorf("unknown resource type: %s", resourceType)
		}
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("FIELD_NOT_ALLOWED: %s", field)
		}
		return nil

	case ContractVersionV2:
		if defaultRegistry == nil {
			return fmt.Errorf("schema registry not initialised")
		}
		if _, ok := defaultRegistry.GetResource(resourceType); !ok {
			return fmt.Errorf("unknown resource type: %s", resourceType)
		}
		normalized := NormalizeField(resourceType, field)
		if !defaultRegistry.IsFieldAllowed(resourceType, intent, normalized) {
			return fmt.Errorf("FIELD_NOT_ALLOWED: %s", field)
		}
		return nil

	default:
		return fmt.Errorf("unsupported contract version: %s", version)
	}
}
