package semantic

import (
	"strings"
	"testing"

	"permission_aware_search/internal/contracts"
)

func TestEnhancedPromptContainsSchema(t *testing.T) {
	builder := NewPromptBuilder(GetDefaultSchemaProvider(), GetDefaultExampleProvider())

	req := AnalyzeRequest{
		Message:         "show open orders with failed payment",
		ContractVersion: contracts.ContractVersionV2,
		ResourceHint:    "order",
	}

	prompt := builder.BuildRewritePrompt(req)

	// Verify prompt contains schema information
	if !strings.Contains(prompt, "Field Schema") {
		t.Error("Prompt should contain field schema section")
	}

	// Verify field definitions are present
	requiredFields := []string{
		"order.number",
		"order.state",
		"shipment.state",
		"payment.state",
		"order.created_at",
	}

	for _, field := range requiredFields {
		if !strings.Contains(prompt, field) {
			t.Errorf("Prompt should contain field definition for %s", field)
		}
	}

	// Verify operator documentation
	operators := []string{"eq", "neq", "gt", "gte", "lt", "lte", "like", "in"}
	for _, op := range operators {
		if !strings.Contains(prompt, op) {
			t.Errorf("Prompt should document operator: %s", op)
		}
	}
}

func TestEnhancedPromptContainsExamples(t *testing.T) {
	builder := NewPromptBuilder(GetDefaultSchemaProvider(), GetDefaultExampleProvider())

	req := AnalyzeRequest{
		Message:         "where is my order",
		ContractVersion: contracts.ContractVersionV2,
		ResourceHint:    "order",
	}

	prompt := builder.BuildRewritePrompt(req)

	// Verify examples section exists
	if !strings.Contains(prompt, "Few-Shot Examples") {
		t.Error("Prompt should contain few-shot examples section")
	}

	// Verify example structure
	if !strings.Contains(prompt, "Example 1") {
		t.Error("Prompt should contain at least one example")
	}

	// Verify example has input/output format
	if !strings.Contains(prompt, "**Input**:") {
		t.Error("Examples should have Input label")
	}

	if !strings.Contains(prompt, "**Output**:") {
		t.Error("Examples should have Output label")
	}
}

func TestEnhancedPromptContainsIntentGuidance(t *testing.T) {
	builder := NewPromptBuilder(GetDefaultSchemaProvider(), GetDefaultExampleProvider())

	req := AnalyzeRequest{
		Message:         "VIP customers",
		ContractVersion: contracts.ContractVersionV2,
		ResourceHint:    "customer",
	}

	prompt := builder.BuildRewritePrompt(req)

	// Verify intent categories are documented
	intents := []string{"wismo", "crm_profile", "returns_refunds"}
	for _, intent := range intents {
		if !strings.Contains(prompt, intent) {
			t.Errorf("Prompt should document intent category: %s", intent)
		}
	}

	// Verify intent-specific guidance
	if !strings.Contains(prompt, "Where Is My Order") {
		t.Error("Prompt should explain WISMO intent")
	}

	if !strings.Contains(prompt, "Customer Relationship Management") {
		t.Error("Prompt should explain CRM intent")
	}
}

func TestEnhancedPromptContainsCriticalRules(t *testing.T) {
	builder := NewPromptBuilder(GetDefaultSchemaProvider(), GetDefaultExampleProvider())

	req := AnalyzeRequest{
		Message:         "delayed orders",
		ContractVersion: contracts.ContractVersionV2,
		ResourceHint:    "order",
	}

	prompt := builder.BuildRewritePrompt(req)

	// Verify critical rules are present
	rules := []string{
		"Field Validation",
		"Operator Matching",
		"Identifier Normalization",
		"Enum Values",
		"Clarification",
		"Evidence",
	}

	for _, rule := range rules {
		if !strings.Contains(prompt, rule) {
			t.Errorf("Prompt should contain rule: %s", rule)
		}
	}

	// Verify normalization examples
	if !strings.Contains(prompt, "ORD-XXXXXX") {
		t.Error("Prompt should show order number format")
	}

	if !strings.Contains(prompt, "TRK-XXXXXXXX") {
		t.Error("Prompt should show tracking ID format")
	}
}

func TestRepairPromptCategorizesErrors(t *testing.T) {
	builder := NewPromptBuilder(GetDefaultSchemaProvider(), GetDefaultExampleProvider())

	req := AnalyzeRequest{
		Message:         "show orders",
		ContractVersion: contracts.ContractVersionV2,
		ResourceHint:    "order",
	}

	previousOutput := map[string]interface{}{
		"intentCategory": "wismo",
		"resourceType":   "order",
	}

	validationErrors := []string{
		"field shipment.carrier not allowed for intent wismo",
		"invalid operator 'contains' for field order.state",
	}

	prompt := builder.BuildRepairPrompt(req, previousOutput, validationErrors)

	// Verify error categorization
	if !strings.Contains(prompt, "Field Not Allowed") {
		t.Error("Repair prompt should categorize field errors")
	}

	if !strings.Contains(prompt, "Invalid Operator") {
		t.Error("Repair prompt should categorize operator errors")
	}

	// Verify targeted guidance
	if !strings.Contains(prompt, "Fix") {
		t.Error("Repair prompt should provide fix guidance")
	}

	// Verify original query is included for context
	if !strings.Contains(prompt, req.Message) {
		t.Error("Repair prompt should include original query")
	}
}

func TestPromptBuilderHandlesDifferentResourceTypes(t *testing.T) {
	builder := NewPromptBuilder(GetDefaultSchemaProvider(), GetDefaultExampleProvider())

	// Test order resource
	orderReq := AnalyzeRequest{
		Message:         "show orders",
		ContractVersion: contracts.ContractVersionV2,
		ResourceHint:    "order",
	}

	orderPrompt := builder.BuildRewritePrompt(orderReq)
	if !strings.Contains(orderPrompt, "order.number") {
		t.Error("Order prompt should contain order fields")
	}

	// Test customer resource
	customerReq := AnalyzeRequest{
		Message:         "show customers",
		ContractVersion: contracts.ContractVersionV2,
		ResourceHint:    "customer",
	}

	customerPrompt := builder.BuildRewritePrompt(customerReq)
	if !strings.Contains(customerPrompt, "customer.vip_tier") {
		t.Error("Customer prompt should contain customer fields")
	}
}

func TestPromptLengthIsReasonable(t *testing.T) {
	builder := NewPromptBuilder(GetDefaultSchemaProvider(), GetDefaultExampleProvider())

	req := AnalyzeRequest{
		Message:         "where is my order",
		ContractVersion: contracts.ContractVersionV2,
		ResourceHint:    "order",
	}

	prompt := builder.BuildRewritePrompt(req)

	// Prompt should be comprehensive but not excessive
	// Typical LLMs can handle 4000-8000 tokens
	// Each character ~= 0.25 tokens, so aim for < 20000 characters
	if len(prompt) > 25000 {
		t.Logf("Warning: Prompt is quite long (%d chars). Consider reducing.", len(prompt))
	}

	// Minimum viable prompt should have core components
	if len(prompt) < 2000 {
		t.Error("Prompt seems too short to be comprehensive")
	}

	t.Logf("Prompt length: %d characters (~%d tokens)", len(prompt), len(prompt)/4)
}
