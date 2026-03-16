package semantic

import (
	"regexp"
	"strings"
)

type SemanticSlots struct {
	OrderState          string             `json:"orderState,omitempty"`
	PaymentState        string             `json:"paymentState,omitempty"`
	HasPaymentConflict  bool               `json:"hasPaymentConflict,omitempty"`
	ShipmentState       string             `json:"shipmentState,omitempty"`
	HasShipmentNegate   bool               `json:"hasShipmentNegate,omitempty"`
	HasShipmentConflict bool               `json:"hasShipmentConflict,omitempty"`
	TimeWindowRecent    bool               `json:"timeWindowRecent,omitempty"`
	HasOrderKeyword     bool               `json:"hasOrderKeyword,omitempty"`
	HasCustomerKeyword  bool               `json:"hasCustomerKeyword,omitempty"`
	MatchedCues         []string           `json:"matchedCues,omitempty"`
	SlotConfidence      map[string]float64 `json:"slotConfidence,omitempty"`
}

var (
	reSemanticWrapper      = regexp.MustCompile(`(?i)searchquery:\s*(.*)$`)
	reSemanticLeadingNoise = regexp.MustCompile(`^[^[:alnum:]@+]+`)
	reSemanticSpaces       = regexp.MustCompile(`\s+`)
)

func normalizeSemanticInput(input string) (string, []string) {
	applied := make([]string, 0, 6)
	n := strings.TrimSpace(input)
	if n == "" {
		return "", applied
	}
	if strings.Contains(strings.ToLower(n), "searchquery:") {
		if m := reSemanticWrapper.FindStringSubmatch(n); len(m) > 1 {
			n = strings.TrimSpace(m[1])
			applied = append(applied, "trim_wrapper")
		}
	}
	n = strings.Trim(n, "\"'")
	cleaned := reSemanticLeadingNoise.ReplaceAllString(n, "")
	if cleaned != n {
		n = cleaned
		applied = append(applied, "strip_leading_symbols")
	}
	normalized := strings.ToLower(strings.TrimSpace(n))
	if normalized != strings.TrimSpace(n) {
		applied = append(applied, "case_normalized_for_match")
	}
	replacements := map[string]string{
		"ordres":                "orders",
		"shwo":                  "show",
		"opne":                  "open",
		"paymnt":                "payment",
		"paymet":                "payment",
		"un-successful":         "unsuccessful",
		"payment unsuccessful":  "payment failed",
		"payment failure":       "payment failed",
		"declined payment":      "payment failed",
		"payment declined":      "payment failed",
		"orders in open status": "orders in open state",
		"order in open status":  "order in open state",
		"state is open":         "open state",
		"show me":               "show",
	}
	for from, to := range replacements {
		if strings.Contains(normalized, from) {
			normalized = strings.ReplaceAll(normalized, from, to)
			applied = append(applied, "phrase_normalized:"+from+"->"+to)
		}
	}
	compacted := reSemanticSpaces.ReplaceAllString(normalized, " ")
	if compacted != normalized {
		normalized = compacted
		applied = append(applied, "collapse_spaces")
	}
	return strings.TrimSpace(normalized), dedupeSemanticStrings(applied)
}

func extractSemanticSlots(lower string) SemanticSlots {
	s := SemanticSlots{
		MatchedCues:    []string{},
		SlotConfidence: map[string]float64{},
	}
	s.HasOrderKeyword = containsAny(lower, "order", "orders", "shipment", "tracking", "payment")
	s.HasCustomerKeyword = containsAny(lower, "customer", "customers", "profile", "vip", "email")
	s.TimeWindowRecent = containsAny(lower,
		"this week", "for the week", "for week", "created this week", "last 7 days",
		"this month", "for the month", "for month", "created this month", "last 30 days", "last 14 days",
	)
	if s.HasOrderKeyword {
		s.MatchedCues = append(s.MatchedCues, "keyword:order_domain")
	}
	if s.HasCustomerKeyword {
		s.MatchedCues = append(s.MatchedCues, "keyword:customer_domain")
	}
	if s.TimeWindowRecent {
		s.MatchedCues = append(s.MatchedCues, "time_window:recent")
		s.SlotConfidence["timeWindowRecent"] = 0.9
	}

	if hasOpenStateCue(lower) {
		s.OrderState = enumRoleValue("order.open_state", "Open")
		s.MatchedCues = append(s.MatchedCues, "order_state:open")
		s.SlotConfidence["orderState"] = 0.95
	}
	if hasPaymentFailedCue(lower) {
		s.PaymentState = enumRoleValue("payment.failed_state", "Failed")
		s.MatchedCues = append(s.MatchedCues, "payment_state:failed")
		s.SlotConfidence["paymentState"] = 0.96
	}
	if containsAny(lower, "payment pending", "pending payment", "unpaid") {
		pendingState := enumRoleValue("payment.pending_state", "Pending")
		if s.PaymentState != "" && s.PaymentState != pendingState {
			s.HasPaymentConflict = true
		}
		s.PaymentState = pendingState
		s.MatchedCues = append(s.MatchedCues, "payment_state:pending")
		s.SlotConfidence["paymentState"] = 0.92
	}
	if hasPaymentPaidCue(lower) {
		paidState := enumRoleValue("payment.paid_state", "Paid")
		if s.PaymentState != "" && s.PaymentState != paidState {
			s.HasPaymentConflict = true
		}
		s.PaymentState = paidState
		s.MatchedCues = append(s.MatchedCues, "payment_state:paid")
		s.SlotConfidence["paymentState"] = 0.94
	}
	if hasNotShippedCue(lower) {
		s.HasShipmentNegate = true
		s.MatchedCues = append(s.MatchedCues, "shipment_negation:not_shipped")
		s.SlotConfidence["hasShipmentNegate"] = 0.97
	}
	hasPositiveShipped := hasExplicitPositiveShippedCue(lower)
	if hasPositiveShipped && !s.HasShipmentNegate {
		s.ShipmentState = enumRoleValue("shipment.shipped_state", "Shipped")
		s.MatchedCues = append(s.MatchedCues, "shipment_state:shipped")
		s.SlotConfidence["shipmentState"] = 0.93
	}
	if s.HasShipmentNegate && hasPositiveShipped {
		s.HasShipmentConflict = true
		s.MatchedCues = append(s.MatchedCues, "shipment_conflict:negate_vs_positive")
		s.SlotConfidence["hasShipmentConflict"] = 0.9
	}
	if s.HasPaymentConflict {
		s.MatchedCues = append(s.MatchedCues, "payment_conflict:multiple_states")
		s.SlotConfidence["hasPaymentConflict"] = 0.9
	}
	s.MatchedCues = dedupeSemanticStrings(s.MatchedCues)
	return s
}

func dedupeSemanticStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func hasExplicitPositiveShippedCue(lower string) bool {
	if containsAny(lower, " but shipped", " and shipped", "already shipped", "is shipped", "are shipped") {
		return true
	}
	if hasNotShippedCue(lower) {
		return false
	}
	return containsAny(lower, "shipped", "shipment shipped")
}
