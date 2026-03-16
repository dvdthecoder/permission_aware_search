package semantic

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"permission_aware_search/internal/contracts"
	"permission_aware_search/internal/schema"
	"permission_aware_search/internal/store"
)

// schemaRegistry is set once at startup via SetSchemaRegistry.
var schemaRegistry *schema.Registry

// SetSchemaRegistry wires the schema registry into the semantic parser.
// When set, enum values in filters come from the registry rather than hardcoded literals.
func SetSchemaRegistry(r *schema.Registry) {
	schemaRegistry = r
}

// enumRoleValue returns the enum value for the given role from the registry,
// falling back to the provided default when no registry is wired (e.g. in isolated tests).
func enumRoleValue(role, fallback string) string {
	if schemaRegistry != nil {
		if v, ok := schemaRegistry.GetEnumRoleValue(role); ok {
			return v
		}
	}
	return fallback
}

// enumRoleField returns the field name for the given enum role from the registry.
// This ensures that if a field is renamed in the schema, the parser follows automatically.
func enumRoleField(role, fallback string) string {
	if schemaRegistry != nil {
		if rm, ok := schemaRegistry.GetEnumRole(role); ok {
			return rm.Field
		}
	}
	return fallback
}

// identifierField returns the primary filter field for the given identifier type from the registry.
func identifierField(identifierType, fallback string) string {
	if schemaRegistry != nil {
		if _, field, ok := schemaRegistry.GetIdentifierByType(identifierType); ok {
			return field
		}
	}
	return fallback
}

var (
	reOrdPrefixed = regexp.MustCompile(`(?i)\bord[-\s]?(\d{3,})\b`)
	reOrdNumeric  = regexp.MustCompile(`(?i)\border\s*#?\s*(\d{4,})\b|#(\d{4,})`)
	reCnum        = regexp.MustCompile(`(?i)\bcust[-\s]?(\d{3,})\b`)
	reTracking    = regexp.MustCompile(`(?i)\btrk[-\s]?(\d{3,})\b`)
	rePaymentRef  = regexp.MustCompile(`(?i)\bpay[-\s]?(\d{3,})\b`)
	rePhone       = regexp.MustCompile(`\+?\d[\d\s\-()]{7,}\d`)
	reNotShipped  = regexp.MustCompile(`(?i)\b(?:not\s+(?:yet\s+)?(?:been\s+)?)shipped\b|\bunshipped\b|\b(?:has|have|had)\s+not\s+been\s+shipped\b|\b(?:hasn't|haven't|hadn't)\s+been\s+shipped\b`)
)

type ParseResult struct {
	Query                store.QueryDSL
	Intent               string
	IntentCategory       string
	IntentSubcategory    string
	ResourceType         string
	NormalizedInput      string
	NormalizationApplied []string
	ExtractedSlots       SemanticSlots
	Confidence           float64
	ClarificationNeeded  bool
	SafeEvidence         []string
}

func ParseNaturalLanguage(message, contractVersion, resourceHint string) ParseResult {
	if contractVersion == "" {
		contractVersion = contracts.ContractVersionV2
	}

	lower, normalizedApplied := normalizeSemanticInput(message)
	slots := extractSemanticSlots(lower)
	intentCategory := classifyIntent(lower)
	intentSubcategory := inferIntentSubcategory(lower, intentCategory)
	resourceType := inferResourceType(lower, resourceHint, intentCategory)

	query := store.QueryDSL{
		ContractVersion: contractVersion,
		IntentCategory:  intentCategory,
		Page:            store.Page{Limit: 20},
	}
	confidence := 0.55
	safeEvidence := []string{}
	if len(normalizedApplied) > 0 {
		safeEvidence = append(safeEvidence, "normalization_applied")
	}

	if days, label, ok := inferRelativeWindow(lower); ok {
		since := relativeTimeAnchor().AddDate(0, 0, -days).Format(time.RFC3339)
		query.Filters = append(query.Filters, store.Filter{Field: createdAtField(resourceType), Op: "gte", Value: since})
		safeEvidence = append(safeEvidence, "time_window:"+label)
		confidence += 0.08
	}

	if ord := extractOrderNumber(message); ord != "" {
		query.Filters = append(query.Filters, store.Filter{Field: identifierField("order_number", "order.number"), Op: "eq", Value: ord})
		safeEvidence = append(safeEvidence, "order_number_lookup")
		confidence += 0.2
	}
	if trk := extractTrackingNumber(message); trk != "" {
		query.Filters = append(query.Filters, store.Filter{Field: identifierField("tracking_id", "shipment.tracking_id"), Op: "eq", Value: trk})
		safeEvidence = append(safeEvidence, "tracking_id_lookup")
		confidence += 0.2
	}
	if pay := extractPaymentReference(message); pay != "" {
		query.Filters = append(query.Filters, store.Filter{Field: identifierField("payment_reference", "payment.reference"), Op: "eq", Value: pay})
		safeEvidence = append(safeEvidence, "payment_reference_lookup")
		confidence += 0.2
	}

	switch intentCategory {
	case contracts.IntentWISMO:
		query.Sort = store.Sort{Field: createdAtField(resourceType), Dir: "desc"}
		confidence = fillWISMOFilters(lower, slots, &query, confidence, &safeEvidence)
		confidence = fillCustomerScopeFilters(lower, resourceType, &query, confidence, &safeEvidence)
	case contracts.IntentCRMProfile:
		query.Sort = store.Sort{Field: createdAtField(resourceType), Dir: "desc"}
		confidence = fillCRMFilters(lower, resourceType, &query, confidence, &safeEvidence)
	case contracts.IntentReturnsRefunds:
		query.Sort = store.Sort{Field: createdAtField(resourceType), Dir: "desc"}
		confidence = fillReturnsFilters(lower, &query, confidence, &safeEvidence)
	default:
		query.Sort = store.Sort{Field: createdAtField(resourceType), Dir: "desc"}
	}

	// Deterministic payment-state conflict guardrail.
	// If both failed and paid cues are present, force clarification instead of emitting contradictory filters.
	if slots.HasPaymentConflict {
		removePaymentStateFilters(&query)
		safeEvidence = append(safeEvidence, "payment_state:conflict_failed_vs_paid")
		confidence = 0.40
	}
	if slots.HasShipmentConflict {
		removeShipmentStateFilters(&query)
		safeEvidence = append(safeEvidence, "shipment_state:conflict_negation_vs_shipped")
		confidence = 0.40
	}

	intent := "search_" + resourceType
	clarification := confidence < 0.66
	return ParseResult{
		Query:                query,
		Intent:               intent,
		IntentCategory:       intentCategory,
		IntentSubcategory:    intentSubcategory,
		ResourceType:         resourceType,
		NormalizedInput:      lower,
		NormalizationApplied: normalizedApplied,
		ExtractedSlots:       slots,
		Confidence:           confidence,
		ClarificationNeeded:  clarification,
		SafeEvidence:         safeEvidence,
	}
}

func inferIntentSubcategory(lower, intentCategory string) string {
	if intentCategory != contracts.IntentWISMO {
		return ""
	}
	if containsAny(lower, "not shipped", "not yet shipped", "unshipped", "hasn't shipped", "has not shipped") {
		return "shipping_tracking"
	}
	if containsAny(lower, "tracking", "track", "where is my package", "where is my order", "shipment") {
		return "shipping_tracking"
	}
	if containsAny(lower, "delay", "delayed", "late", "eta", "backlog", "weather disruption") {
		return "delivery_exception"
	}
	if containsAny(lower, "carrier", "courier", "ups", "fedex", "dhl", "usps") {
		return "carrier_issue"
	}
	if containsAny(lower, "fulfillment", "warehouse", "processing") {
		return "fulfillment_delay"
	}
	return "shipping_tracking"
}

func classifyIntent(lower string) string {
	if containsAny(lower,
		"calendar", "poster", "flyer", "brochure", "banner", "business cards",
		"material", "color", "style", "product catalog",
	) {
		return contracts.IntentUnsupported
	}

	if containsAny(lower,
		"return", "refund", "eligible", "rma", "pickup", "refund issued", "refund transactions",
	) {
		return contracts.IntentReturnsRefunds
	}

	// Temporal order listing defaults to WISMO for operational support workflows.
	if isTemporalOrderListing(lower) {
		return contracts.IntentWISMO
	}

	if containsAny(lower,
		"where is my order", "wismo", "tracking", "shipped", "package", "delay", "status of order",
		"timeline of events", "fulfillment", "courier", "shipment", "delivered", "processing",
		"payment succeed", "payment fail", "payment captured", "fraud", "chargebacks", "failed payment",
		"payment failure", "payment unsuccessful", "declined payment", "open state", "state open",
		"payment pending", "pending payment", "unpaid", "awaiting payment", "payment due",
		"investigation report", "trace order", "what went wrong", "open order", "open orders",
	) {
		return contracts.IntentWISMO
	}

	// CRM intent is identity/profile gated to avoid broad "orders for ..." misclassification.
	if hasCRMIdentityCue(lower) && containsAny(lower,
		"last 3 orders", "recent orders", "orders for", "orders placed by", "order history", "order history for customer",
		"lifetime value", "segment", "vip", "profile", "history",
		"tickets", "account notes", "loyalty", "customer intelligence", "repeat buyer", "support",
	) {
		return contracts.IntentCRMProfile
	}
	return contracts.IntentDefault
}

func isTemporalOrderListing(lower string) bool {
	if !containsAny(lower, "order", "orders") {
		return false
	}
	return containsAny(lower,
		"this week", "for the week", "for week", "created this week",
		"this month", "for the month", "for month", "created this month",
		"last 30 days", "last 7 days", "created last",
	)
}

func inferRelativeWindow(lower string) (int, string, bool) {
	// Week patterns
	if containsAny(lower, "this week", "for the week", "for week", "created this week", "last 7 days") {
		return 7, "week", true
	}
	// Month patterns
	if containsAny(lower, "this month", "for the month", "for month", "created this month", "last 30 days") {
		return 30, "month", true
	}
	// Explicit "last N days" patterns
	if containsAny(lower, "last 14 days") {
		return 14, "14days", true
	}
	return 0, "", false
}

func hasCRMIdentityCue(lower string) bool {
	if extractEmail(lower) != "" || extractPhone(lower) != "" || extractCustomerNumber(lower) != "" {
		return true
	}
	return containsAny(lower,
		"customer id", "customer number", "customer #",
		"vip", "segment", "profile", "account notes", "tickets", "support history",
	)
}

func inferResourceType(lower, hint, intent string) string {
	if hint == "order" || hint == "customer" {
		return hint
	}
	if containsAny(lower, "order", "orders", "package", "tracking", "shipment", "refund", "return", "fulfillment") {
		return "order"
	}
	if containsAny(lower, "customer", "email", "phone", "vip", "profile", "account") {
		return "customer"
	}
	if intent == contracts.IntentWISMO || intent == contracts.IntentReturnsRefunds {
		return "order"
	}
	return "order"
}

func fillWISMOFilters(lower string, slots SemanticSlots, q *store.QueryDSL, confidence float64, evidence *[]string) float64 {
	if strings.Contains(lower, "tracking") {
		*evidence = append(*evidence, "wismo:tracking")
		confidence += 0.12
	}
	openState := enumRoleValue("order.open_state", "Open")
	if slots.OrderState == openState {
		q.Filters = append(q.Filters, store.Filter{Field: enumRoleField("order.open_state", "order.state"), Op: "eq", Value: openState})
		*evidence = append(*evidence, "order_state:"+openState+" (open_state_cue)")
		confidence += 0.14
	}
	if slots.HasShipmentNegate {
		applyNotShippedFilters(q)
		*evidence = append(*evidence, "shipment_state:NOT_ShippedOrDeliveredOrReady")
		confidence += 0.12
	} else {
		shippedState := enumRoleValue("shipment.shipped_state", "Shipped")
		if slots.ShipmentState == shippedState {
			q.Filters = append(q.Filters, store.Filter{Field: enumRoleField("shipment.shipped_state", "shipment.state"), Op: "eq", Value: shippedState})
			*evidence = append(*evidence, "shipment_state:"+shippedState)
			confidence += 0.12
		}
	}
	if strings.Contains(lower, "delay") || strings.Contains(lower, "delayed") {
		delayedState := enumRoleValue("shipment.delayed_state", "Delayed")
		q.Filters = append(q.Filters, store.Filter{Field: enumRoleField("shipment.delayed_state", "shipment.state"), Op: "eq", Value: delayedState})
		*evidence = append(*evidence, "shipment_state:"+delayedState)
		confidence += 0.12
	}
	if slots.PaymentState != "" && !slots.HasPaymentConflict {
		q.Filters = append(q.Filters, store.Filter{Field: enumRoleField("payment.failed_state", "payment.state"), Op: "eq", Value: slots.PaymentState})
		*evidence = append(*evidence, "payment_state:"+slots.PaymentState)
		confidence += 0.12
	}
	if strings.Contains(lower, "courier") {
		confidence += 0.08
		*evidence = append(*evidence, "logistics:courier")
	}
	if strings.Contains(lower, "fulfillment") {
		confidence += 0.08
		*evidence = append(*evidence, "fulfillment:investigation")
	}
	return confidence
}

func fillCRMFilters(lower, resourceType string, q *store.QueryDSL, confidence float64, evidence *[]string) float64 {
	confidence = fillCustomerScopeFilters(lower, resourceType, q, confidence, evidence)

	if strings.Contains(lower, "vip") || strings.Contains(lower, "loyalty") {
		baseTier := enumRoleValue("customer.vip_base_tier", "silver")
		q.Filters = append(q.Filters, store.Filter{Field: enumRoleField("customer.vip_base_tier", "customer.vip_tier"), Op: "neq", Value: baseTier})
		*evidence = append(*evidence, "vip_profile_filter")
		confidence += 0.12
	}
	if strings.Contains(lower, "verified") {
		q.Filters = append(q.Filters, store.Filter{Field: "customer.is_email_verified", Op: "eq", Value: 1})
		*evidence = append(*evidence, "email_verified_filter")
		confidence += 0.1
	}
	return confidence
}

func fillCustomerScopeFilters(lower, resourceType string, q *store.QueryDSL, confidence float64, evidence *[]string) float64 {
	if email := extractEmail(lower); email != "" {
		field := "customer.email"
		ev := "customer_email_lookup"
		if resourceType == "order" {
			field = "order.customer_email"
			ev = "order_customer_email_lookup"
		}
		q.Filters = append(q.Filters, store.Filter{Field: field, Op: "eq", Value: email})
		*evidence = append(*evidence, ev)
		confidence += 0.22
	}
	if cnum := extractCustomerNumber(lower); cnum != "" {
		if resourceType == "customer" {
			q.Filters = append(q.Filters, store.Filter{Field: "customer.number", Op: "eq", Value: cnum})
			*evidence = append(*evidence, "customer_number_lookup")
			confidence += 0.2
		} else {
			if cid := customerIDFromCustomerNumber(cnum); cid != "" {
				q.Filters = append(q.Filters, store.Filter{Field: "order.customer_id", Op: "eq", Value: cid})
				*evidence = append(*evidence, "order_customer_id_from_customer_number")
				confidence += 0.2
			} else {
				*evidence = append(*evidence, "customer_number_context")
				confidence += 0.08
			}
		}
	}
	if phone := extractPhone(lower); phone != "" {
		*evidence = append(*evidence, "phone_lookup_context")
		confidence += 0.1
		_ = phone
	}
	return confidence
}

func fillReturnsFilters(lower string, q *store.QueryDSL, confidence float64, evidence *[]string) float64 {
	if strings.Contains(lower, "eligible") {
		eligibleVal := enumRoleValue("return.eligible_true", "true")
		q.Filters = append(q.Filters, store.Filter{Field: enumRoleField("return.eligible_true", "return.eligible"), Op: "eq", Value: eligibleVal})
		*evidence = append(*evidence, "return_eligible:"+eligibleVal)
		confidence += 0.14
	}
	if strings.Contains(lower, "refund") {
		notInitiated := enumRoleValue("refund.not_initiated_state", "NotInitiated")
		q.Filters = append(q.Filters, store.Filter{Field: enumRoleField("refund.not_initiated_state", "refund.status"), Op: "neq", Value: notInitiated})
		*evidence = append(*evidence, "refund_status_non_default")
		confidence += 0.12
	}
	if strings.Contains(lower, "return") {
		notRequested := enumRoleValue("return.not_requested_state", "NotRequested")
		q.Filters = append(q.Filters, store.Filter{Field: enumRoleField("return.not_requested_state", "return.status"), Op: "neq", Value: notRequested})
		*evidence = append(*evidence, "return_status_non_default")
		confidence += 0.12
	}
	return confidence
}

func containsAny(in string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(in, term) {
			return true
		}
	}
	return false
}

func hasPaymentFailedCue(lower string) bool {
	return containsAny(lower,
		"payment fail",
		"payment failed",
		"failed payment",
		"payment failure",
		"payment unsuccessful",
		"declined payment",
		"payment declined",
	)
}

func hasPaymentPaidCue(lower string) bool {
	return containsAny(lower, "payment succeed", "payment captured", "payment successful", "payment paid")
}

func hasOpenStateCue(lower string) bool {
	return containsAny(lower,
		"open order",
		"open orders",
		"orders in open state",
		"order in open state",
		"orders open state",
		"order state open",
		"state is open",
		"state open",
		"still in processing",
	)
}

func relativeTimeAnchor() time.Time {
	raw := strings.TrimSpace(os.Getenv("DEMO_TIME_ANCHOR"))
	if raw == "" {
		return time.Now().UTC()
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Now().UTC()
	}
	return ts.UTC()
}

func hasNotShippedCue(lower string) bool {
	return reNotShipped.MatchString(lower)
}

func applyNotShippedFilters(q *store.QueryDSL) {
	if q == nil {
		return
	}
	type denyEntry struct{ field, value string }
	var deny []denyEntry
	if schemaRegistry != nil {
		for _, rm := range schemaRegistry.GetEnumRolesByPrefix("shipment.not_shipped_deny.") {
			deny = append(deny, denyEntry{field: rm.Field, value: rm.Value})
		}
	}
	if len(deny) == 0 {
		deny = []denyEntry{
			{field: "shipment.state", value: "Shipped"},
			{field: "shipment.state", value: "Delivered"},
			{field: "shipment.state", value: "Ready"},
		}
	}
	for _, d := range deny {
		if !hasStateNegation(q.Filters, d.field, d.value) {
			q.Filters = append(q.Filters, store.Filter{Field: d.field, Op: "neq", Value: d.value})
		}
	}
}

func removePaymentStateFilters(q *store.QueryDSL) {
	if q == nil || len(q.Filters) == 0 {
		return
	}
	out := make([]store.Filter, 0, len(q.Filters))
	for _, f := range q.Filters {
		if f.Field == "payment.state" {
			continue
		}
		out = append(out, f)
	}
	q.Filters = out
}

func removeShipmentStateFilters(q *store.QueryDSL) {
	if q == nil || len(q.Filters) == 0 {
		return
	}
	out := make([]store.Filter, 0, len(q.Filters))
	for _, f := range q.Filters {
		if f.Field == "shipment.state" {
			continue
		}
		out = append(out, f)
	}
	q.Filters = out
}

func hasStateNegation(filters []store.Filter, field, value string) bool {
	for _, f := range filters {
		if f.Field == field && strings.EqualFold(f.Op, "neq") && strings.EqualFold(strings.TrimSpace(toString(f.Value)), value) {
			return true
		}
	}
	return false
}

func toString(v interface{}) string {
	return fmt.Sprint(v)
}

func extractPaymentReference(message string) string {
	if m := rePaymentRef.FindStringSubmatch(strings.TrimSpace(message)); len(m) >= 2 {
		digits := m[1]
		if len(digits) > 8 {
			digits = digits[len(digits)-8:]
		}
		for len(digits) < 8 {
			digits = "0" + digits
		}
		return "PAY-" + digits
	}
	return ""
}

func createdAtField(resourceType string) string {
	if schemaRegistry != nil {
		if f, ok := schemaRegistry.GetDefaultSortField(resourceType); ok {
			return f
		}
	}
	if resourceType == "customer" {
		return "customer.created_at"
	}
	return "order.created_at"
}

func extractOrderNumber(in string) string {
	if m := reOrdPrefixed.FindStringSubmatch(in); len(m) > 1 {
		return "ORD-" + leftPad(m[1], 6)
	}
	if m := reOrdNumeric.FindStringSubmatch(in); len(m) > 0 {
		for _, candidate := range m[1:] {
			if candidate != "" {
				return "ORD-" + leftPad(candidate, 6)
			}
		}
	}
	return ""
}

func extractTrackingNumber(in string) string {
	if m := reTracking.FindStringSubmatch(in); len(m) > 1 {
		return "TRK-" + leftPad(m[1], 8)
	}
	return ""
}

func extractCustomerNumber(in string) string {
	if m := reCnum.FindStringSubmatch(in); len(m) > 1 {
		return "CUST-" + leftPad(m[1], 6)
	}
	return ""
}

func extractPhone(in string) string {
	return rePhone.FindString(in)
}

func extractEmail(in string) string {
	parts := strings.Fields(in)
	for _, part := range parts {
		clean := strings.Trim(part, ",.!?\"'()[]{}")
		if strings.Contains(clean, "@") && strings.Contains(clean, ".") {
			return strings.ToLower(clean)
		}
	}
	return ""
}

func leftPad(num string, width int) string {
	n := strings.TrimSpace(num)
	if len(n) >= width {
		return n
	}
	return strings.Repeat("0", width-len(n)) + n
}

func customerIDFromCustomerNumber(cnum string) string {
	parts := strings.Split(strings.ToUpper(cnum), "-")
	if len(parts) != 2 {
		return ""
	}
	num := strings.TrimSpace(parts[1])
	if num == "" {
		return ""
	}
	if len(num) > 5 {
		num = num[len(num)-5:]
	}
	return "cust-" + leftPad(num, 5)
}
