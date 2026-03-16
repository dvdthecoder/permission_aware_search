package semantic

import (
	"fmt"
	"strings"

	"permission_aware_search/internal/contracts"
)

// DemoEnhancedPrompt shows an example of the enhanced prompt for documentation purposes.
// Run this to see what the SLM actually receives.
func DemoEnhancedPrompt() {
	builder := NewPromptBuilder(GetDefaultSchemaProvider(), GetDefaultExampleProvider())

	req := AnalyzeRequest{
		Message:         "show open orders with failed payment",
		ContractVersion: contracts.ContractVersionV2,
		ResourceHint:    "order",
	}

	prompt := builder.BuildRewritePrompt(req)

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("ENHANCED SLM PROMPT EXAMPLE")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()
	fmt.Printf("Query: %s\n", req.Message)
	fmt.Println()
	fmt.Println("Generated Prompt:")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println(prompt)
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("\nPrompt Length: %d characters (~%d tokens)\n", len(prompt), len(prompt)/4)
}

// DemoRepairPrompt shows an example of the enhanced repair prompt.
func DemoRepairPrompt() {
	builder := NewPromptBuilder(GetDefaultSchemaProvider(), GetDefaultExampleProvider())

	req := AnalyzeRequest{
		Message:         "show orders",
		ContractVersion: contracts.ContractVersionV2,
		ResourceHint:    "order",
	}

	previousOutput := map[string]interface{}{
		"intent":         "search_order",
		"intentCategory": "wismo",
		"resourceType":   "order",
		"query": map[string]interface{}{
			"filters": []map[string]interface{}{
				{"field": "shipment.carrier", "op": "eq", "value": "UPS"},
				{"field": "order.state", "op": "contains", "value": "Open"},
			},
		},
	}

	validationErrors := []string{
		"field shipment.carrier not allowed for intent wismo in contract v2",
		"invalid operator 'contains' for field order.state - use eq, neq, in",
	}

	prompt := builder.BuildRepairPrompt(req, previousOutput, validationErrors)

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("ENHANCED REPAIR PROMPT EXAMPLE")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()
	fmt.Printf("Query: %s\n", req.Message)
	fmt.Printf("Errors: %v\n", validationErrors)
	fmt.Println()
	fmt.Println("Generated Repair Prompt:")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println(prompt)
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("\nPrompt Length: %d characters (~%d tokens)\n", len(prompt), len(prompt)/4)
}
