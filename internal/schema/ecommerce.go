// Package schema provides the single source of truth for all data-shape
// knowledge: field names, native column mappings, enum values, identifier
// patterns, and semantic enum-role bindings.
//
// To adapt the system to a different schema, create a new *Definition
// function (e.g. MyCompanyDefinition()) and pass it to New() at startup.
// No other files need to change.
package schema

import (
	"regexp"
	"strings"
)

// EcommerceDefinition returns the complete schema definition for the demo
// e-commerce domain.  This is the ONLY place in the codebase that contains:
//   - field names and their native SQL column names
//   - valid enum values per field
//   - identifier regex patterns and normalization logic
//   - semantic enum-role bindings used by the query parser
//   - V2 intent-scope allowlists (encoded in Field.IntentScopes)
//   - V1 searchable field sets
func EcommerceDefinition() Definition {
	return Definition{
		Resources: []Resource{
			orderResource(),
			customerResource(),
		},
	}
}

// allIntents returns the full set of intent scope strings.
func allIntents() []string {
	return []string{"default", "wismo", "crm_profile", "returns_refunds"}
}

func orderResource() Resource {
	return Resource{
		Name:        "order",
		TableName:   "orders_docs",
		DefaultSort: "order.created_at",

		// V1 legacy: field alias names accepted by contract v1
		V1SearchableFields: []string{
			"id", "status", "created_at", "total_amount", "customer_id",
		},

		Fields: []Field{
			{
				Name: "order.id", NativeColumn: "id",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq"},
				IntentScopes: allIntents(),
				Description:  "Internal order ID",
			},
			{
				Name: "order.number", NativeColumn: "order_number",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq", "like"},
				IntentScopes: allIntents(),
				Description:  "Order identifier",
				Example:      "ORD-123456",
			},
			{
				Name: "order.customer_id", NativeColumn: "customer_id",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq"},
				IntentScopes: []string{"default", "wismo", "crm_profile"},
				Description:  "Internal customer ID",
				Example:      "cust-12345",
			},
			{
				Name: "order.customer_email", NativeColumn: "customer_email",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq", "like"},
				IntentScopes: []string{"default", "wismo", "crm_profile"},
				Description:  "Customer email address",
				Example:      "customer@example.com",
			},
			{
				Name: "order.state", NativeColumn: "order_state",
				Type: "enum", Sortable: true, Filterable: true,
				EnumValues:   []string{"Open", "Confirmed", "Complete", "Cancelled"},
				Operators:    []string{"eq", "neq", "in"},
				IntentScopes: []string{"default", "wismo", "crm_profile"},
				Description:  "Current order state",
				Example:      "Open",
			},
			{
				Name: "order.created_at", NativeColumn: "created_at",
				Type: "timestamp", Sortable: true, Filterable: true,
				Operators:    []string{"gt", "gte", "lt", "lte"},
				IntentScopes: allIntents(),
				Description:  "Order creation timestamp",
				Example:      "2025-03-01T12:00:00Z",
			},
			{
				Name: "order.completed_at", NativeColumn: "completed_at",
				Type: "timestamp", Sortable: true, Filterable: true,
				Operators:    []string{"gt", "gte", "lt", "lte"},
				IntentScopes: []string{"default", "wismo", "crm_profile", "returns_refunds"},
				Description:  "Order completion timestamp",
				Example:      "2025-03-05T14:00:00Z",
			},
			{
				Name: "order.total_cent_amount", NativeColumn: "total_cent_amount",
				Type: "int", Sortable: true, Filterable: true,
				Operators:    []string{"eq", "gt", "gte", "lt", "lte"},
				IntentScopes: []string{"default", "crm_profile"},
				Description:  "Order total in cents",
				Example:      "9999",
			},
			{
				Name: "order.currency_code", NativeColumn: "currency_code",
				Type: "string", Sortable: false, Filterable: true,
				Operators:    []string{"eq"},
				IntentScopes: []string{"default"},
				Description:  "Currency code",
				Example:      "USD",
			},
			{
				Name: "shipment.state", NativeColumn: "shipment_state",
				Type: "enum", Sortable: true, Filterable: true,
				EnumValues:   []string{"Pending", "Shipped", "Delivered", "Delayed", "Ready"},
				Operators:    []string{"eq", "neq", "in"},
				IntentScopes: []string{"wismo"},
				Description:  "Current shipment status",
				Example:      "Shipped",
			},
			{
				Name: "shipment.tracking_id", NativeColumn: "tracking_id",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq", "like"},
				IntentScopes: []string{"wismo"},
				Description:  "Carrier tracking number",
				Example:      "TRK-12345678",
			},
			{
				Name: "payment.state", NativeColumn: "payment_state",
				Type: "enum", Sortable: false, Filterable: true,
				EnumValues:   []string{"Pending", "Paid", "Failed", "Refunded"},
				Operators:    []string{"eq", "neq", "in"},
				IntentScopes: []string{"default", "wismo", "crm_profile"},
				Description:  "Payment status",
				Example:      "Paid",
			},
			{
				Name: "payment.reference", NativeColumn: "payment_reference",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq"},
				IntentScopes: []string{"default", "wismo", "returns_refunds"},
				Description:  "Payment transaction reference",
				Example:      "PAY-87654321",
			},
			{
				Name: "return.eligible", NativeColumn: "return_eligible",
				Type: "string", Sortable: false, Filterable: true,
				EnumValues:   []string{"true", "false"},
				Operators:    []string{"eq"},
				IntentScopes: []string{"returns_refunds"},
				Description:  "Return eligibility flag",
				Example:      "true",
			},
			{
				Name: "return.status", NativeColumn: "return_status",
				Type: "enum", Sortable: false, Filterable: true,
				EnumValues:   []string{"NotRequested", "Requested", "Approved", "Completed"},
				Operators:    []string{"eq", "neq"},
				IntentScopes: []string{"returns_refunds"},
				Description:  "Return request status",
				Example:      "Requested",
			},
			{
				Name: "refund.status", NativeColumn: "refund_status",
				Type: "enum", Sortable: false, Filterable: true,
				EnumValues:   []string{"NotInitiated", "Pending", "Processed", "Failed"},
				Operators:    []string{"eq", "neq"},
				IntentScopes: []string{"returns_refunds"},
				Description:  "Refund processing status",
				Example:      "Pending",
			},
			{
				Name: "order.legacy_status", NativeColumn: "status",
				Type: "string", Sortable: false, Filterable: true,
				Operators:    []string{"eq"},
				IntentScopes: []string{"default"},
				Description:  "Legacy order status (V1)",
				Example:      "Open",
			},
			{
				Name: "order.legacy_total_amount", NativeColumn: "total_amount",
				Type: "int", Sortable: false, Filterable: true,
				Operators:    []string{"eq", "gt", "gte", "lt", "lte"},
				IntentScopes: []string{"default"},
				Description:  "Legacy total amount (V1)",
				Example:      "9999",
			},
			{
				Name: "order.region", NativeColumn: "region",
				Type: "string", Sortable: false, Filterable: true,
				Operators:    []string{"eq"},
				IntentScopes: []string{"default"},
				Description:  "Order region",
				Example:      "EU",
			},
		},

		Aliases: []Alias{
			{Input: "order.id", Resolves: "order.id"},
			{Input: "id", Resolves: "order.id"},
			{Input: "order.number", Resolves: "order.number"},
			{Input: "order_number", Resolves: "order.number"},
			{Input: "order.customer_id", Resolves: "order.customer_id"},
			{Input: "customer_id", Resolves: "order.customer_id"},
			{Input: "order.customer_email", Resolves: "order.customer_email"},
			{Input: "customer_email", Resolves: "order.customer_email"},
			{Input: "order.state", Resolves: "order.state"},
			{Input: "order_state", Resolves: "order.state"},
			{Input: "shipment.state", Resolves: "shipment.state"},
			{Input: "shipment_state", Resolves: "shipment.state"},
			{Input: "payment.state", Resolves: "payment.state"},
			{Input: "payment_state", Resolves: "payment.state"},
			{Input: "order.created_at", Resolves: "order.created_at"},
			{Input: "created_at", Resolves: "order.created_at"},
			{Input: "order.completed_at", Resolves: "order.completed_at"},
			{Input: "completed_at", Resolves: "order.completed_at"},
			{Input: "shipment.tracking_id", Resolves: "shipment.tracking_id"},
			{Input: "tracking_id", Resolves: "shipment.tracking_id"},
			{Input: "payment.reference", Resolves: "payment.reference"},
			{Input: "payment_reference", Resolves: "payment.reference"},
			{Input: "order.total_cent_amount", Resolves: "order.total_cent_amount"},
			{Input: "total_cent_amount", Resolves: "order.total_cent_amount"},
			{Input: "order.currency_code", Resolves: "order.currency_code"},
			{Input: "currency_code", Resolves: "order.currency_code"},
			{Input: "return.eligible", Resolves: "return.eligible"},
			{Input: "return_eligible", Resolves: "return.eligible"},
			{Input: "return.status", Resolves: "return.status"},
			{Input: "return_status", Resolves: "return.status"},
			{Input: "refund.status", Resolves: "refund.status"},
			{Input: "refund_status", Resolves: "refund.status"},
			{Input: "order.legacy_status", Resolves: "order.legacy_status"},
			{Input: "status", Resolves: "order.legacy_status"},
			{Input: "order.legacy_total_amount", Resolves: "order.legacy_total_amount"},
			{Input: "total_amount", Resolves: "order.legacy_total_amount"},
			{Input: "order.region", Resolves: "order.region"},
			{Input: "region", Resolves: "order.region"},
		},

		Identifiers: []IdentifierPattern{
			{
				Name:          "ord_default",
				IdentifierType: "order_number",
				Regex:         regexp.MustCompile(`(?i)\bORD-\d{6}\b|\bord-\d{5}\b`),
				ResourceType:  "order",
				PrimaryField:  "order.number",
			},
			{
				Name:          "trk_default",
				IdentifierType: "tracking_id",
				Regex:         regexp.MustCompile(`(?i)\bTRK-\d{8}\b|\bTB-TRK-\d{6}\b`),
				ResourceType:  "order",
				PrimaryField:  "shipment.tracking_id",
			},
			{
				Name:          "pay_default",
				IdentifierType: "payment_reference",
				Regex:         regexp.MustCompile(`(?i)\bPAY-\d{8}\b`),
				ResourceType:  "order",
				PrimaryField:  "payment.reference",
			},
			{
				Name:          "pix_pattern",
				IdentifierType: "order_number",
				Regex:         regexp.MustCompile(`(?i)\bPIX-[A-Z]{3}-\d+\b`),
				ResourceType:  "order",
				PrimaryField:  "order.number",
			},
			{
				Name:          "efl_pattern",
				IdentifierType: "order_number",
				Regex:         regexp.MustCompile(`(?i)\bEFL-[A-Z]{3}-\d+\b`),
				ResourceType:  "order",
				PrimaryField:  "order.number",
			},
			{
				Name:          "prefixed_numeric",
				IdentifierType: "order_number",
				Regex:         regexp.MustCompile(`(?i)\b[a-z]\d{6,}\b`),
				ResourceType:  "order",
				PrimaryField:  "order.number",
			},
			{
				Name:          "numeric_long",
				IdentifierType: "order_number",
				Regex:         regexp.MustCompile(`\b\d{6,}\b`),
				ResourceType:  "order",
				PrimaryField:  "order.number",
			},
		},

		EnumRoles: []EnumRoleMapping{
			// Order states
			{Role: "order.open_state", Field: "order.state", Value: "Open"},
			{Role: "order.confirmed_state", Field: "order.state", Value: "Confirmed"},
			{Role: "order.complete_state", Field: "order.state", Value: "Complete"},
			{Role: "order.cancelled_state", Field: "order.state", Value: "Cancelled"},
			// Shipment states
			{Role: "shipment.pending_state", Field: "shipment.state", Value: "Pending"},
			{Role: "shipment.shipped_state", Field: "shipment.state", Value: "Shipped"},
			{Role: "shipment.delivered_state", Field: "shipment.state", Value: "Delivered"},
			{Role: "shipment.delayed_state", Field: "shipment.state", Value: "Delayed"},
			{Role: "shipment.ready_state", Field: "shipment.state", Value: "Ready"},
			// "Not yet delivered" deny-list — used by applyNotShippedFilters
			{Role: "shipment.not_shipped_deny.shipped", Field: "shipment.state", Value: "Shipped"},
			{Role: "shipment.not_shipped_deny.delivered", Field: "shipment.state", Value: "Delivered"},
			{Role: "shipment.not_shipped_deny.ready", Field: "shipment.state", Value: "Ready"},
			// Payment states
			{Role: "payment.pending_state", Field: "payment.state", Value: "Pending"},
			{Role: "payment.paid_state", Field: "payment.state", Value: "Paid"},
			{Role: "payment.failed_state", Field: "payment.state", Value: "Failed"},
			{Role: "payment.refunded_state", Field: "payment.state", Value: "Refunded"},
			// Return states
			{Role: "return.not_requested_state", Field: "return.status", Value: "NotRequested"},
			{Role: "return.requested_state", Field: "return.status", Value: "Requested"},
			{Role: "return.approved_state", Field: "return.status", Value: "Approved"},
			{Role: "return.completed_state", Field: "return.status", Value: "Completed"},
			// Refund states
			{Role: "refund.not_initiated_state", Field: "refund.status", Value: "NotInitiated"},
			{Role: "refund.pending_state", Field: "refund.status", Value: "Pending"},
			{Role: "refund.processed_state", Field: "refund.status", Value: "Processed"},
			{Role: "refund.failed_state", Field: "refund.status", Value: "Failed"},
			// Return eligibility
			{Role: "return.eligible_true", Field: "return.eligible", Value: "true"},
		},
	}
}

func customerResource() Resource {
	return Resource{
		Name:        "customer",
		TableName:   "customers_docs",
		DefaultSort: "customer.created_at",

		V1SearchableFields: []string{
			"id", "name", "tier", "region", "created_at",
		},

		Fields: []Field{
			{
				Name: "customer.id", NativeColumn: "id",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq"},
				IntentScopes: allIntents(),
				Description:  "Internal customer ID",
			},
			{
				Name: "customer.number", NativeColumn: "customer_number",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq", "like"},
				IntentScopes: allIntents(),
				Description:  "Customer identifier",
				Example:      "CUST-123456",
			},
			{
				Name: "customer.email", NativeColumn: "email",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq", "like"},
				IntentScopes: allIntents(),
				Description:  "Customer email address",
				Example:      "customer@example.com",
			},
			{
				Name: "customer.first_name", NativeColumn: "first_name",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq", "like"},
				IntentScopes: allIntents(),
				Description:  "Customer first name",
				Example:      "Jane",
			},
			{
				Name: "customer.last_name", NativeColumn: "last_name",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq", "like"},
				IntentScopes: allIntents(),
				Description:  "Customer last name",
				Example:      "Smith",
			},
			{
				Name: "customer.is_email_verified", NativeColumn: "is_email_verified",
				Type: "int", Sortable: false, Filterable: true,
				Operators:    []string{"eq"},
				IntentScopes: []string{"crm_profile"},
				Description:  "Email verification status (1=verified, 0=unverified)",
				Example:      "1",
			},
			{
				Name: "customer.created_at", NativeColumn: "created_at",
				Type: "timestamp", Sortable: true, Filterable: true,
				Operators:    []string{"gt", "gte", "lt", "lte"},
				IntentScopes: []string{"crm_profile"},
				Description:  "Customer account creation timestamp",
				Example:      "2024-01-15T10:30:00Z",
			},
			{
				Name: "customer.group", NativeColumn: "customer_group",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq", "like"},
				IntentScopes: []string{"default", "crm_profile"},
				Description:  "Customer segment group",
				Example:      "enterprise",
			},
			{
				Name: "customer.vip_tier", NativeColumn: "vip_tier",
				Type: "enum", Sortable: true, Filterable: true,
				EnumValues:   []string{"gold", "platinum", "diamond", "silver"},
				Operators:    []string{"eq", "neq", "in"},
				IntentScopes: []string{"crm_profile"},
				Description:  "VIP loyalty tier",
				Example:      "gold",
			},
			{
				Name: "customer.legacy_name", NativeColumn: "name",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq", "like"},
				IntentScopes: []string{"default"},
				Description:  "Legacy customer name (V1)",
				Example:      "Jane Smith",
			},
			{
				Name: "customer.legacy_tier", NativeColumn: "tier",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq"},
				IntentScopes: []string{"default"},
				Description:  "Legacy customer tier (V1)",
				Example:      "gold",
			},
			{
				Name: "customer.region", NativeColumn: "region",
				Type: "string", Sortable: true, Filterable: true,
				Operators:    []string{"eq"},
				IntentScopes: []string{"default"},
				Description:  "Customer region",
				Example:      "EU",
			},
		},

		Aliases: []Alias{
			{Input: "customer.id", Resolves: "customer.id"},
			{Input: "id", Resolves: "customer.id"},
			{Input: "customer.number", Resolves: "customer.number"},
			{Input: "customer_number", Resolves: "customer.number"},
			{Input: "customer.email", Resolves: "customer.email"},
			{Input: "email", Resolves: "customer.email"},
			{Input: "customer.first_name", Resolves: "customer.first_name"},
			{Input: "first_name", Resolves: "customer.first_name"},
			{Input: "customer.last_name", Resolves: "customer.last_name"},
			{Input: "last_name", Resolves: "customer.last_name"},
			{Input: "customer.is_email_verified", Resolves: "customer.is_email_verified"},
			{Input: "is_email_verified", Resolves: "customer.is_email_verified"},
			{Input: "customer.created_at", Resolves: "customer.created_at"},
			{Input: "created_at", Resolves: "customer.created_at"},
			{Input: "customer.group", Resolves: "customer.group"},
			{Input: "customer_group", Resolves: "customer.group"},
			{Input: "customer.vip_tier", Resolves: "customer.vip_tier"},
			{Input: "vip_tier", Resolves: "customer.vip_tier"},
			{Input: "customer.legacy_name", Resolves: "customer.legacy_name"},
			{Input: "name", Resolves: "customer.legacy_name"},
			{Input: "customer.legacy_tier", Resolves: "customer.legacy_tier"},
			{Input: "tier", Resolves: "customer.legacy_tier"},
			{Input: "customer.region", Resolves: "customer.region"},
			{Input: "region", Resolves: "customer.region"},
		},

		Identifiers: []IdentifierPattern{
			{
				Name:          "cust_default",
				IdentifierType: "customer_number",
				Regex:         regexp.MustCompile(`(?i)\bCUST-\d{6}\b|\bcust-\d{5}\b|\bTB-CUST-\d{5}\b`),
				ResourceType:  "customer",
				PrimaryField:  "customer.number",
				SecondaryFields: []SecondaryLookup{
					{
						ResourceType: "order",
						Field:        "order.customer_id",
						Transformer:  custNumberToCustomerID,
						Confidence:   0.85,
					},
				},
			},
			{
				Name:          "email_default",
				IdentifierType: "email",
				Regex:         regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`),
				ResourceType:  "customer",
				PrimaryField:  "customer.email",
				SecondaryFields: []SecondaryLookup{
					{
						ResourceType: "order",
						Field:        "order.customer_email",
						Confidence:   0.9,
					},
				},
			},
			{
				Name:          "phone_default",
				IdentifierType: "phone",
				Regex:         regexp.MustCompile(`\+?\d[\d\s\-()]{7,}\d`),
				ResourceType:  "customer",
				PrimaryField:  "customer.email", // phones resolve via email lookup
			},
		},

		EnumRoles: []EnumRoleMapping{
			{Role: "customer.vip_base_tier", Field: "customer.vip_tier", Value: "silver"},
			{Role: "customer.vip_gold_tier", Field: "customer.vip_tier", Value: "gold"},
			{Role: "customer.vip_platinum_tier", Field: "customer.vip_tier", Value: "platinum"},
			{Role: "customer.vip_diamond_tier", Field: "customer.vip_tier", Value: "diamond"},
		},
	}
}

// custNumberToCustomerID converts a normalised CUST-XXXXXX value to the
// internal cust-XXXXX format used in orders.customer_id.
// This transformation is schema-specific and therefore lives here.
func custNumberToCustomerID(in string) string {
	upper := strings.ToUpper(in)
	if len(upper) >= 5 && strings.HasPrefix(upper, "CUST-") {
		return "cust-" + upper[len(upper)-5:]
	}
	return in
}
