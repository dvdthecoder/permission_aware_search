package semantic

import (
	"fmt"
	"strings"

	"permission_aware_search/internal/contracts"
)

// PromptBuilder builds enhanced prompts for SLM query rewriting.
type PromptBuilder struct {
	schemaProvider  SchemaProvider
	exampleProvider ExampleProvider
}

// NewPromptBuilder creates a new prompt builder with schema and example providers.
func NewPromptBuilder(schemaProvider SchemaProvider, exampleProvider ExampleProvider) *PromptBuilder {
	return &PromptBuilder{
		schemaProvider:  schemaProvider,
		exampleProvider: exampleProvider,
	}
}

// BuildRewritePrompt creates a comprehensive prompt for query rewriting.
func (pb *PromptBuilder) BuildRewritePrompt(req AnalyzeRequest) string {
	resourceType := req.ResourceHint
	if resourceType == "" {
		resourceType = "order" // Default to order
	}

	contractVersion := req.ContractVersion
	if contractVersion == "" {
		contractVersion = contracts.ContractVersionV2
	}

	schema := pb.schemaProvider.GetSchema(resourceType, contractVersion)

	// Get examples - try to match intent hint if provided
	intentHint := inferIntentHint(req.Message)
	examples := pb.exampleProvider.GetExamples(intentHint)
	if len(examples) > 6 {
		examples = examples[:6] // Limit to 6 examples to avoid token bloat
	}

	prompt := fmt.Sprintf(`You are an expert query rewriter for an e-commerce support search system.

**Task**: Convert natural language queries from support agents into structured search filters.

**Input Query**: %s
**Resource Type**: %s (order | customer)
**Contract Version**: %s

---

## Field Schema for %s

%s

---

## Valid Operators

- **eq** (equals): For exact matches on strings, enums, IDs
- **neq** (not equals): For exclusions
- **gt, gte, lt, lte** (comparison): For numbers and timestamps
- **like** (partial match): For text search (use with %% wildcard)
- **in** (set membership): For matching against multiple values

---

## Intent Categories

1. **wismo** (Where Is My Order)
   - Focus: shipment tracking, delivery status, order timeline
   - Allowed fields: order.number, shipment.*, order.state, payment.state, tracking_id
   - Common patterns: "where is my order", "tracking", "delayed", "not shipped"

2. **crm_profile** (Customer Relationship Management)
   - Focus: customer history, profile data, segmentation
   - Allowed fields: customer.*, order.customer_email, order.customer_id
   - Common patterns: "customer orders", "VIP customers", "order history"

3. **returns_refunds** (Returns & Refunds)
   - Focus: return eligibility, refund status
   - Allowed fields: return.*, refund.*, order.number
   - Common patterns: "eligible for return", "pending refunds", "return status"

4. **default**
   - Generic queries that don't fit other categories
   - Full field access (subject to contract version)

---

%s

---

## Output Format

Return ONLY valid JSON with this exact structure (no markdown, no explanation):

{
  "intent": "search_order" | "search_customer",
  "intentCategory": "wismo" | "crm_profile" | "returns_refunds" | "default",
  "intentSubcategory": "shipping_tracking" | "delivery_exception" | "carrier_issue" | "fulfillment_delay" | "",
  "resourceType": "order" | "customer",
  "confidence": 0.0-1.0,
  "clarificationNeeded": true | false,
  "safeEvidence": ["reason_code_1", "reason_code_2"],
  "query": {
    "contractVersion": "%s",
    "intentCategory": "wismo" | "crm_profile" | "returns_refunds" | "default",
    "filters": [
      {"field": "field.name", "op": "eq|neq|gt|gte|lt|lte|like|in", "value": "value"}
    ],
    "sort": {"field": "field.name", "dir": "asc" | "desc"},
    "page": {"limit": 20, "offset": 0}
  }
}

---

## Critical Rules

1. **Field Validation**: Only use fields allowed for the intent category (check "Intent Scope" column in schema)
2. **Operator Matching**: Use correct operators for field types:
   - Enums: eq, neq, in
   - Strings: eq, like
   - Timestamps: gt, gte, lt, lte
   - IDs: eq only
3. **Identifier Normalization**:
   - Order numbers: ORD-XXXXXX (6 digits, zero-padded)
   - Tracking IDs: TRK-XXXXXXXX (8 digits, zero-padded)
   - Customer numbers: CUST-XXXXXX (6 digits, zero-padded)
   - Payment refs: PAY-XXXXXXXX (8 digits, zero-padded)
4. **Enum Values**: Only use values listed in schema EnumValues
5. **Clarification**: Set clarificationNeeded=true if query is ambiguous or lacks context
6. **Evidence**: Include safeEvidence array with reason codes explaining filter choices
7. **Timestamps**: Use ISO8601 format (2025-03-14T12:00:00Z)
8. **Negation**: For "not shipped", use multiple neq filters for Shipped, Delivered, Ready
9. **Time Windows**:
   - "this week", "created this week", "last 7 days" → order.created_at gte (7 days ago)
   - "this month", "created this month", "last 30 days" → order.created_at gte (30 days ago)
   - "yesterday" → order.created_at between start and end of yesterday
   - Always use ISO8601 format for date values

---

Now rewrite the input query following the schema, examples, and rules above:`,
		req.Message,
		resourceType,
		contractVersion,
		resourceType,
		schema.RenderFieldList(),
		pb.exampleProvider.RenderExamples(examples),
		contractVersion,
	)

	return prompt
}

// BuildRepairPrompt creates a targeted repair prompt based on validation errors.
func (pb *PromptBuilder) BuildRepairPrompt(
	req AnalyzeRequest,
	previousOutput map[string]interface{},
	validationErrors []string,
) string {
	errorCategories := categorizeErrors(validationErrors)

	prompt := fmt.Sprintf(`Your previous query rewrite had validation errors. Fix them as follows:

**Original Query**: %s

**Your Previous Output (INVALID)**:
%s

**Validation Errors**:
%s

---

## Error-Specific Fixes

%s

---

## Critical Reminders

1. Only use fields allowed for the chosen intentCategory
2. Only use operators: eq, neq, gt, gte, lt, lte, like, in
3. Ensure enum values match exactly (case-sensitive)
4. Normalize all identifiers (ORD-XXXXXX, TRK-XXXXXXXX, CUST-XXXXXX)
5. Timestamps must be ISO8601 format

---

Return the corrected JSON (no markdown, no explanation):`,
		req.Message,
		toJSONString(previousOutput),
		strings.Join(validationErrors, "\n"),
		buildErrorGuidance(errorCategories),
	)

	return prompt
}

// inferIntentHint tries to guess the intent from the query for example selection.
func inferIntentHint(message string) string {
	lower := strings.ToLower(message)

	if containsAny(lower, "return", "refund", "eligible", "rma") {
		return contracts.IntentReturnsRefunds
	}

	if containsAny(lower, "customer", "vip", "profile", "segment") {
		return contracts.IntentCRMProfile
	}

	if containsAny(lower, "tracking", "shipped", "delayed", "where is", "wismo", "package") {
		return contracts.IntentWISMO
	}

	return contracts.IntentDefault
}

// categorizeErrors groups validation errors by type.
func categorizeErrors(errors []string) map[string][]string {
	categories := map[string][]string{
		"field_not_allowed":     {},
		"invalid_operator":      {},
		"invalid_enum_value":    {},
		"intent_mismatch":       {},
		"resource_mismatch":     {},
		"invalid_format":        {},
		"missing_required":      {},
	}

	for _, err := range errors {
		errLower := strings.ToLower(err)

		if strings.Contains(errLower, "field") && strings.Contains(errLower, "not allowed") {
			categories["field_not_allowed"] = append(categories["field_not_allowed"], err)
		} else if strings.Contains(errLower, "operator") || strings.Contains(errLower, "op") {
			categories["invalid_operator"] = append(categories["invalid_operator"], err)
		} else if strings.Contains(errLower, "enum") || strings.Contains(errLower, "value") {
			categories["invalid_enum_value"] = append(categories["invalid_enum_value"], err)
		} else if strings.Contains(errLower, "intent") {
			categories["intent_mismatch"] = append(categories["intent_mismatch"], err)
		} else if strings.Contains(errLower, "resource") {
			categories["resource_mismatch"] = append(categories["resource_mismatch"], err)
		} else if strings.Contains(errLower, "format") {
			categories["invalid_format"] = append(categories["invalid_format"], err)
		} else {
			categories["missing_required"] = append(categories["missing_required"], err)
		}
	}

	return categories
}

// buildErrorGuidance generates specific guidance based on error categories.
func buildErrorGuidance(categories map[string][]string) string {
	var b strings.Builder

	if len(categories["field_not_allowed"]) > 0 {
		b.WriteString("### Field Not Allowed Errors\n")
		for _, err := range categories["field_not_allowed"] {
			b.WriteString(fmt.Sprintf("- %s\n", err))
		}
		b.WriteString("\n**Fix**: Remove these fields OR change intentCategory to one that allows them.\n")
		b.WriteString("- shipment.* fields → use intentCategory: \"wismo\"\n")
		b.WriteString("- customer.vip_tier → use intentCategory: \"crm_profile\"\n")
		b.WriteString("- return.*, refund.* → use intentCategory: \"returns_refunds\"\n\n")
	}

	if len(categories["invalid_operator"]) > 0 {
		b.WriteString("### Invalid Operator Errors\n")
		for _, err := range categories["invalid_operator"] {
			b.WriteString(fmt.Sprintf("- %s\n", err))
		}
		b.WriteString("\n**Fix**: Use only: eq, neq, gt, gte, lt, lte, like, in\n\n")
	}

	if len(categories["invalid_enum_value"]) > 0 {
		b.WriteString("### Invalid Enum Value Errors\n")
		for _, err := range categories["invalid_enum_value"] {
			b.WriteString(fmt.Sprintf("- %s\n", err))
		}
		b.WriteString("\n**Fix**: Check schema for valid enum values (case-sensitive):\n")
		b.WriteString("- order.state: Open, Confirmed, Complete, Cancelled\n")
		b.WriteString("- shipment.state: Pending, Shipped, Delivered, Delayed, Ready\n")
		b.WriteString("- payment.state: Pending, Paid, Failed, Refunded\n\n")
	}

	if len(categories["intent_mismatch"]) > 0 {
		b.WriteString("### Intent Mismatch Errors\n")
		for _, err := range categories["intent_mismatch"] {
			b.WriteString(fmt.Sprintf("- %s\n", err))
		}
		b.WriteString("\n**Fix**: Ensure intentCategory matches query.intentCategory\n\n")
	}

	if len(categories["resource_mismatch"]) > 0 {
		b.WriteString("### Resource Mismatch Errors\n")
		for _, err := range categories["resource_mismatch"] {
			b.WriteString(fmt.Sprintf("- %s\n", err))
		}
		b.WriteString("\n**Fix**: WISMO and returns_refunds require resourceType: \"order\"\n\n")
	}

	if b.Len() == 0 {
		b.WriteString("Review all errors and ensure output matches the schema exactly.\n")
	}

	return b.String()
}

// toJSONString converts a map to formatted JSON string.
func toJSONString(data map[string]interface{}) string {
	if data == nil {
		return "{}"
	}
	// Format as compact JSON
	var b strings.Builder
	b.WriteString("{\n")
	first := true
	for k, v := range data {
		if !first {
			b.WriteString(",\n")
		}
		first = false
		b.WriteString(fmt.Sprintf("  \"%s\": %v", k, formatValue(v)))
	}
	b.WriteString("\n}")
	return b.String()
}

func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("\"%s\"", val)
	case map[string]interface{}:
		return toJSONString(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
