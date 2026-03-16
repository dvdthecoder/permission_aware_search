package identifier

import (
	"strings"

	"permission_aware_search/internal/schema"
)

// defaultSchemaRegistry is set once at startup via SetDefaultSchemaRegistry.
var defaultSchemaRegistry *schema.Registry

// SetDefaultSchemaRegistry wires the schema registry for identifier resolution.
// When set, the switch-based hardcoded type→field mapping is replaced by
// registry lookups.  Falls back to the switch if nil (backward compat for tests).
func SetDefaultSchemaRegistry(r *schema.Registry) {
	defaultSchemaRegistry = r
}

func BuildResolutionPlan(input, resourceHint string) ResolutionPlan {
	return BuildResolutionPlanWithConfig(input, "", resourceHint, nil, QueryShapeThresholds{})
}

func BuildResolutionPlanWithConfig(input, tenantID, resourceHint string, registry *PatternRegistry, thresholds QueryShapeThresholds) ResolutionPlan {
	analysis := AnalyzeQuery(input, tenantID, registry, thresholds)
	detected := analysis.Detected
	shouldUse := ShouldUseFastPath(analysis.NormalizedInput, detected)
	groups := make([]GroupSpec, 0, len(detected)*2)
	isTypeahead := analysis.QueryShape == ShapeTypeahead

	// For numeric-only inputs without prefixes, do multi-field prefix search
	isNumericOnly := isOnlyNumeric(input)

	for _, d := range detected {
		op := "eq"
		val := d.NormalizedValue
		if isTypeahead || d.Type == TypeUnknownToken {
			op = "like"
			val = d.NormalizedValue + "%"
		}

		// Special handling for numeric-only inputs: search across all identifier fields
		if isNumericOnly {
			// Add searches for all possible identifier types
			groups = append(groups,
				GroupSpec{ResourceType: "order", MatchField: "order.number", Operator: "like", Value: input + "%", Confidence: 0.6},
				GroupSpec{ResourceType: "order", MatchField: "shipment.tracking_id", Operator: "like", Value: input + "%", Confidence: 0.5},
				GroupSpec{ResourceType: "order", MatchField: "payment.reference", Operator: "like", Value: input + "%", Confidence: 0.5},
				GroupSpec{ResourceType: "customer", MatchField: "customer.number", Operator: "like", Value: input + "%", Confidence: 0.5},
			)
			// Don't process the standard switch for numeric-only inputs
			continue
		}

		if defaultSchemaRegistry != nil {
			// Registry path: data shape comes from schema, not from code
			resource, field, ok := defaultSchemaRegistry.GetIdentifierByType(string(d.Type))
			if ok {
				groups = append(groups, GroupSpec{ResourceType: resource, MatchField: field, Operator: op, Value: val, Confidence: d.Confidence})
				for _, sec := range defaultSchemaRegistry.GetSecondaryLookups(string(d.Type)) {
					secVal := d.NormalizedValue
					if sec.Transformer != nil {
						secVal = sec.Transformer(d.NormalizedValue)
					}
					secOp := "eq"
					if op == "like" {
						secOp = "like"
						secVal = secVal + "%"
					}
					groups = append(groups, GroupSpec{ResourceType: sec.ResourceType, MatchField: sec.Field, Operator: secOp, Value: secVal, Confidence: sec.Confidence})
				}
			} else if d.Type == TypeUnknownToken && d.NormalizedValue != "" {
				// Unknown tokens fan out to all identifier fields
				for _, ip := range defaultSchemaRegistry.GetAllIdentifierPatterns() {
					groups = append(groups, GroupSpec{ResourceType: ip.ResourceType, MatchField: ip.PrimaryField, Operator: "like", Value: d.NormalizedValue + "%", Confidence: 0.4})
				}
			}
		} else {
			// Fallback: legacy hardcoded switch (keeps existing tests working without registry)
			switch d.Type {
			case TypeOrderNumber:
				groups = append(groups, GroupSpec{ResourceType: "order", MatchField: "order.number", Operator: op, Value: val, Confidence: d.Confidence})
			case TypeTrackingID:
				groups = append(groups, GroupSpec{ResourceType: "order", MatchField: "shipment.tracking_id", Operator: op, Value: val, Confidence: d.Confidence})
			case TypePaymentRef:
				groups = append(groups, GroupSpec{ResourceType: "order", MatchField: "payment.reference", Operator: op, Value: val, Confidence: d.Confidence})
			case TypeCustomerNumber:
				groups = append(groups, GroupSpec{ResourceType: "customer", MatchField: "customer.number", Operator: op, Value: val, Confidence: d.Confidence})
				groups = append(groups, GroupSpec{ResourceType: "order", MatchField: "order.customer_id", Operator: op, Value: customerIDFromNumber(d.NormalizedValue), Confidence: 0.85})
			case TypeEmail:
				emailOp := op
				emailVal := val
				if emailOp == "like" && len(d.NormalizedValue) < 3 {
					continue
				}
				if emailOp == "eq" {
					emailVal = d.NormalizedValue
				}
				groups = append(groups, GroupSpec{ResourceType: "customer", MatchField: "customer.email", Operator: emailOp, Value: emailVal, Confidence: d.Confidence})
				groups = append(groups, GroupSpec{ResourceType: "order", MatchField: "order.customer_email", Operator: emailOp, Value: emailVal, Confidence: d.Confidence})
			case TypeUnknownToken:
				if d.NormalizedValue != "" {
					groups = append(groups,
						GroupSpec{ResourceType: "order", MatchField: "order.number", Operator: "like", Value: d.NormalizedValue + "%", Confidence: 0.4},
						GroupSpec{ResourceType: "order", MatchField: "shipment.tracking_id", Operator: "like", Value: d.NormalizedValue + "%", Confidence: 0.4},
						GroupSpec{ResourceType: "order", MatchField: "payment.reference", Operator: "like", Value: d.NormalizedValue + "%", Confidence: 0.4},
						GroupSpec{ResourceType: "customer", MatchField: "customer.number", Operator: "like", Value: d.NormalizedValue + "%", Confidence: 0.4},
						GroupSpec{ResourceType: "customer", MatchField: "customer.email", Operator: "like", Value: d.NormalizedValue + "%", Confidence: 0.4},
					)
				}
			}
		}
	}

	if resourceHint == "order" || resourceHint == "customer" {
		filtered := make([]GroupSpec, 0, len(groups))
		for _, g := range groups {
			if g.ResourceType == resourceHint {
				filtered = append(filtered, g)
			}
		}
		if len(filtered) > 0 {
			groups = filtered
		}
	}
	return ResolutionPlan{
		ShouldUseFastPath:        shouldUse && len(groups) > 0,
		Detected:                 detected,
		Groups:                   dedupeGroups(groups),
		QueryShape:               analysis.QueryShape,
		NormalizationApplied:     analysis.NormalizationApplied,
		IdentifierPatternMatched: analysis.IdentifierPatternMatched,
		NormalizedInput:          analysis.NormalizedInput,
	}
}

func dedupeGroups(in []GroupSpec) []GroupSpec {
	seen := map[string]struct{}{}
	out := make([]GroupSpec, 0, len(in))
	for _, g := range in {
		key := g.ResourceType + "|" + g.MatchField + "|" + g.Operator + "|" + g.Value
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, g)
	}
	return out
}

func customerIDFromNumber(in string) string {
	if len(in) >= len("CUST-")+6 && in[:5] == "CUST-" {
		return "cust-" + in[len(in)-5:]
	}
	return in
}

// isOnlyNumeric checks if the input contains only digits (no prefixes like ORD-, TRK-, etc.)
func isOnlyNumeric(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return false
	}
	for _, ch := range trimmed {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
