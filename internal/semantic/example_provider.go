package semantic

import (
	"encoding/json"
	"fmt"
	"strings"

	"permission_aware_search/internal/contracts"
	"permission_aware_search/internal/store"
)

// ExampleProvider provides few-shot examples for query rewriting.
type ExampleProvider interface {
	GetExamples(intentHint string) []QueryExample
	RenderExamples(examples []QueryExample) string
}

// QueryExample represents a single few-shot example.
type QueryExample struct {
	Query       string
	Intent      string
	Category    string
	Subcategory string
	Resource    string
	Filters     []store.Filter
	Sort        store.Sort
	Evidence    []string
	Confidence  float64
}

// StaticExampleProvider provides a static set of curated examples.
type StaticExampleProvider struct {
	examples map[string][]QueryExample
}

// NewStaticExampleProvider creates an example provider with curated examples.
func NewStaticExampleProvider() *StaticExampleProvider {
	return &StaticExampleProvider{
		examples: map[string][]QueryExample{
			contracts.IntentWISMO: {
				{
					Query:       "where is order ORD-123456",
					Intent:      "search_order",
					Category:    contracts.IntentWISMO,
					Subcategory: "shipping_tracking",
					Resource:    "order",
					Filters: []store.Filter{
						{Field: "order.number", Op: "eq", Value: "ORD-123456"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"order_number_lookup", "wismo:tracking"},
					Confidence: 0.95,
				},
				{
					Query:       "show open orders with failed payment",
					Intent:      "search_order",
					Category:    contracts.IntentWISMO,
					Subcategory: "",
					Resource:    "order",
					Filters: []store.Filter{
						{Field: "order.state", Op: "eq", Value: "Open"},
						{Field: "payment.state", Op: "eq", Value: "Failed"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"order_state:Open", "payment_state:Failed"},
					Confidence: 0.9,
				},
				{
					Query:       "orders not shipped this week",
					Intent:      "search_order",
					Category:    contracts.IntentWISMO,
					Subcategory: "shipping_tracking",
					Resource:    "order",
					Filters: []store.Filter{
						{Field: "shipment.state", Op: "neq", Value: "Shipped"},
						{Field: "shipment.state", Op: "neq", Value: "Delivered"},
						{Field: "shipment.state", Op: "neq", Value: "Ready"},
						{Field: "order.created_at", Op: "gte", Value: "2025-03-07T00:00:00Z"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"shipment_state:NOT_Shipped", "time_window:week"},
					Confidence: 0.88,
				},
				{
					Query:       "tracking TRK-12345678",
					Intent:      "search_order",
					Category:    contracts.IntentWISMO,
					Subcategory: "shipping_tracking",
					Resource:    "order",
					Filters: []store.Filter{
						{Field: "shipment.tracking_id", Op: "eq", Value: "TRK-12345678"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"tracking_id_lookup", "wismo:tracking"},
					Confidence: 0.95,
				},
				{
					Query:       "delayed shipments",
					Intent:      "search_order",
					Category:    contracts.IntentWISMO,
					Subcategory: "delivery_exception",
					Resource:    "order",
					Filters: []store.Filter{
						{Field: "shipment.state", Op: "eq", Value: "Delayed"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"shipment_state:Delayed"},
					Confidence: 0.92,
				},
				{
					Query:       "orders with payment pending",
					Intent:      "search_order",
					Category:    contracts.IntentWISMO,
					Subcategory: "",
					Resource:    "order",
					Filters: []store.Filter{
						{Field: "payment.state", Op: "eq", Value: "Pending"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"payment_state:Pending"},
					Confidence: 0.9,
				},
			},
			contracts.IntentCRMProfile: {
				{
					Query:    "orders for customer aster@example.com",
					Intent:   "search_order",
					Category: contracts.IntentCRMProfile,
					Resource: "order",
					Filters: []store.Filter{
						{Field: "order.customer_email", Op: "eq", Value: "aster@example.com"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"customer_email_lookup"},
					Confidence: 0.92,
				},
				{
					Query:    "VIP customers",
					Intent:   "search_customer",
					Category: contracts.IntentCRMProfile,
					Resource: "customer",
					Filters: []store.Filter{
						{Field: "customer.vip_tier", Op: "neq", Value: "silver"},
					},
					Sort:       store.Sort{Field: "customer.created_at", Dir: "desc"},
					Evidence:   []string{"vip_profile_filter"},
					Confidence: 0.85,
				},
				{
					Query:    "customer CUST-123456",
					Intent:   "search_customer",
					Category: contracts.IntentCRMProfile,
					Resource: "customer",
					Filters: []store.Filter{
						{Field: "customer.number", Op: "eq", Value: "CUST-123456"},
					},
					Sort:       store.Sort{Field: "customer.created_at", Dir: "desc"},
					Evidence:   []string{"customer_number_lookup"},
					Confidence: 0.95,
				},
				{
					Query:    "orders this month by customer aster@example.com",
					Intent:   "search_order",
					Category: contracts.IntentCRMProfile,
					Resource: "order",
					Filters: []store.Filter{
						{Field: "order.customer_email", Op: "eq", Value: "aster@example.com"},
						{Field: "order.created_at", Op: "gte", Value: "2025-03-01T00:00:00Z"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"customer_email_lookup", "time_window:month"},
					Confidence: 0.9,
				},
			},
			contracts.IntentReturnsRefunds: {
				{
					Query:    "orders eligible for return",
					Intent:   "search_order",
					Category: contracts.IntentReturnsRefunds,
					Resource: "order",
					Filters: []store.Filter{
						{Field: "return.eligible", Op: "eq", Value: "true"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"return_eligible:true"},
					Confidence: 0.9,
				},
				{
					Query:    "pending refunds",
					Intent:   "search_order",
					Category: contracts.IntentReturnsRefunds,
					Resource: "order",
					Filters: []store.Filter{
						{Field: "refund.status", Op: "eq", Value: "Pending"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"refund_status:Pending"},
					Confidence: 0.92,
				},
				{
					Query:    "approved returns",
					Intent:   "search_order",
					Category: contracts.IntentReturnsRefunds,
					Resource: "order",
					Filters: []store.Filter{
						{Field: "return.status", Op: "eq", Value: "Approved"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"return_status:Approved"},
					Confidence: 0.92,
				},
			},
			contracts.IntentDefault: {
				{
					Query:    "orders for the week",
					Intent:   "search_order",
					Category: contracts.IntentDefault,
					Resource: "order",
					Filters: []store.Filter{
						{Field: "order.created_at", Op: "gte", Value: "2025-03-07T00:00:00Z"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"time_window:week"},
					Confidence: 0.85,
				},
				{
					Query:    "orders created this week",
					Intent:   "search_order",
					Category: contracts.IntentWISMO,
					Resource: "order",
					Filters: []store.Filter{
						{Field: "order.created_at", Op: "gte", Value: "2025-03-07T00:00:00Z"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"time_window:week"},
					Confidence: 0.9,
				},
				{
					Query:    "orders created this month",
					Intent:   "search_order",
					Category: contracts.IntentWISMO,
					Resource: "order",
					Filters: []store.Filter{
						{Field: "order.created_at", Op: "gte", Value: "2025-03-01T00:00:00Z"},
					},
					Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
					Evidence:   []string{"time_window:month"},
					Confidence: 0.9,
				},
			},
		},
	}
}

func (p *StaticExampleProvider) GetExamples(intentHint string) []QueryExample {
	if examples, ok := p.examples[intentHint]; ok {
		return examples
	}
	// If no specific intent, return a mix from all categories
	var allExamples []QueryExample
	for _, examples := range p.examples {
		if len(allExamples) < 10 {
			allExamples = append(allExamples, examples...)
		}
	}
	return allExamples
}

func (p *StaticExampleProvider) RenderExamples(examples []QueryExample) string {
	if len(examples) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n## Few-Shot Examples\n\n")
	b.WriteString("Use these examples to understand the expected output format:\n")

	for i, ex := range examples {
		b.WriteString(fmt.Sprintf("\n### Example %d\n", i+1))
		b.WriteString(fmt.Sprintf("**Input**: \"%s\"\n\n", ex.Query))
		b.WriteString("**Output**:\n```json\n")

		output := map[string]interface{}{
			"intent":              ex.Intent,
			"intentCategory":      ex.Category,
			"intentSubcategory":   ex.Subcategory,
			"resourceType":        ex.Resource,
			"confidence":          ex.Confidence,
			"clarificationNeeded": false,
			"safeEvidence":        ex.Evidence,
			"query": map[string]interface{}{
				"contractVersion": "v2",
				"intentCategory":  ex.Category,
				"filters":         ex.Filters,
				"sort":            ex.Sort,
				"page": map[string]interface{}{
					"limit":  20,
					"offset": 0,
				},
			},
		}

		jsonBytes, _ := json.MarshalIndent(output, "", "  ")
		b.WriteString(string(jsonBytes))
		b.WriteString("\n```\n")
	}

	return b.String()
}

// GetDefaultExampleProvider returns the default example provider for the system.
func GetDefaultExampleProvider() ExampleProvider {
	return NewStaticExampleProvider()
}
