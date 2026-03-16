package semantic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"permission_aware_search/internal/store"
)

func TestSLMLocalIntentMapping(t *testing.T) {
	a := NewSLMLocalAnalyzer()
	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "Where is my order ORD-1234?"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.IntentCategory != "wismo" {
		t.Fatalf("expected wismo category, got %s", res.IntentCategory)
	}
	if res.ResourceType != "order" {
		t.Fatalf("expected order resource, got %s", res.ResourceType)
	}
}

func TestSLMLocalOrdersForEmailIsNotAmbiguous(t *testing.T) {
	a := NewSLMLocalAnalyzer()
	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show orders for aster@example.com"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.ResourceType != "order" {
		t.Fatalf("expected order resource type, got %s", res.ResourceType)
	}
	if res.IntentCategory != "crm_profile" {
		t.Fatalf("expected crm_profile intent, got %s", res.IntentCategory)
	}
	if res.ClarificationNeeded {
		t.Fatalf("expected non-ambiguous parse for orders-by-email query")
	}
	found := false
	for _, f := range res.Query.Filters {
		if f.Field == "order.customer_email" && f.Op == "eq" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected customer_email filter in generated query")
	}
}

func TestSLMLocalWISMOByCustomerEmailAddsCustomerFilter(t *testing.T) {
	a := NewSLMLocalAnalyzer()
	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show open orders this week by customer aster@example.com"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.IntentCategory != "wismo" {
		t.Fatalf("expected wismo intent, got %s", res.IntentCategory)
	}
	foundEmail := false
	for _, f := range res.Query.Filters {
		if f.Field == "order.customer_email" && f.Op == "eq" {
			foundEmail = true
			break
		}
	}
	if !foundEmail {
		t.Fatalf("expected customer_email filter in wismo query, got filters: %+v", res.Query.Filters)
	}
}

func TestSLMLocalMonthOrdersRoutesToWISMO(t *testing.T) {
	a := NewSLMLocalAnalyzer()
	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show orders for the month"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.IntentCategory != "wismo" {
		t.Fatalf("expected wismo intent, got %s", res.IntentCategory)
	}
	foundCreated := false
	for _, f := range res.Query.Filters {
		if f.Field == "order.created_at" && f.Op == "gte" {
			foundCreated = true
			break
		}
	}
	if !foundCreated {
		t.Fatalf("expected created_at gte filter for month query")
	}
}

func TestTemporalFilterUsesDemoTimeAnchorWhenSet(t *testing.T) {
	t.Setenv("DEMO_TIME_ANCHOR", "2025-02-15T00:00:00Z")
	res := ParseNaturalLanguage("show open orders this week", "v2", "")
	foundCreated := false
	for _, f := range res.Query.Filters {
		if f.Field == "order.created_at" && f.Op == "gte" {
			foundCreated = true
			if s, ok := f.Value.(string); !ok || !strings.HasPrefix(s, "2025-02-08") {
				t.Fatalf("expected created_at anchored to 2025-02-15, got %v", f.Value)
			}
			break
		}
	}
	if !foundCreated {
		t.Fatalf("expected created_at temporal filter")
	}
}

func TestSLMLocalMonthOrdersByCustomerRoutesToWISMOAndCustomerFilter(t *testing.T) {
	a := NewSLMLocalAnalyzer()
	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show orders for the month by customer aster@example.com"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.IntentCategory != "wismo" {
		t.Fatalf("expected wismo intent, got %s", res.IntentCategory)
	}
	foundEmail := false
	for _, f := range res.Query.Filters {
		if f.Field == "order.customer_email" && f.Op == "eq" {
			foundEmail = true
			break
		}
	}
	if !foundEmail {
		t.Fatalf("expected customer_email filter in generated query")
	}
}

func TestSLMLocalOpenOrdersPaymentPendingAddsPendingFilter(t *testing.T) {
	a := NewSLMLocalAnalyzer()
	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "Show open orders that have payment pending"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.IntentCategory != "wismo" {
		t.Fatalf("expected wismo intent, got %s", res.IntentCategory)
	}
	foundOpenState := false
	foundPendingPayment := false
	for _, f := range res.Query.Filters {
		if f.Field == "order.state" && f.Op == "eq" && f.Value == "Open" {
			foundOpenState = true
		}
		if f.Field == "payment.state" && f.Op == "eq" && f.Value == "Pending" {
			foundPendingPayment = true
		}
	}
	if !foundOpenState {
		t.Fatalf("expected order_state open filter")
	}
	if !foundPendingPayment {
		t.Fatalf("expected payment_state Pending filter")
	}
}

func TestSLMLocalFailedPaymentPhraseMapsToFailedFilter(t *testing.T) {
	a := NewSLMLocalAnalyzer()
	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show order with failed payment"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.IntentCategory != "wismo" {
		t.Fatalf("expected wismo intent, got %s", res.IntentCategory)
	}
	if res.ClarificationNeeded {
		t.Fatalf("expected non-ambiguous parse for failed payment phrase")
	}
	foundFailed := false
	for _, f := range res.Query.Filters {
		if f.Field == "payment.state" && f.Op == "eq" && f.Value == "Failed" {
			foundFailed = true
			break
		}
	}
	if !foundFailed {
		t.Fatalf("expected payment.state eq Failed filter, got %+v", res.Query.Filters)
	}
}

func TestSLMLocalOpenStatePhraseMapsToOpenFilter(t *testing.T) {
	a := NewSLMLocalAnalyzer()
	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show orders in open state"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.IntentCategory != "wismo" {
		t.Fatalf("expected wismo intent, got %s", res.IntentCategory)
	}
	if res.ClarificationNeeded {
		t.Fatalf("expected non-ambiguous parse for open state phrase")
	}
	foundOpen := false
	for _, f := range res.Query.Filters {
		if f.Field == "order.state" && f.Op == "eq" && f.Value == "Open" {
			foundOpen = true
			break
		}
	}
	if !foundOpen {
		t.Fatalf("expected order.state eq Open filter, got %+v", res.Query.Filters)
	}
}

func TestSLMLocalNoisyOpenStatePhraseNormalizes(t *testing.T) {
	a := NewSLMLocalAnalyzer()
	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "shwo ordres in opne state"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.IntentCategory != "wismo" {
		t.Fatalf("expected wismo intent, got %s", res.IntentCategory)
	}
	foundOpen := false
	for _, f := range res.Query.Filters {
		if f.Field == "order.state" && f.Op == "eq" && f.Value == "Open" {
			foundOpen = true
			break
		}
	}
	if !foundOpen {
		t.Fatalf("expected order.state eq Open filter, got %+v", res.Query.Filters)
	}
}

func TestSLMLocalPaymentStateConflictRequiresClarification(t *testing.T) {
	a := NewSLMLocalAnalyzer()
	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show orders with failed payment and payment captured"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if !res.ClarificationNeeded {
		t.Fatalf("expected clarification for conflicting payment cues")
	}
	for _, f := range res.Query.Filters {
		if f.Field == "payment.state" {
			t.Fatalf("did not expect payment.state filter under conflict, got %+v", res.Query.Filters)
		}
	}
}

func TestSLMLocalShipmentConflictRequiresClarification(t *testing.T) {
	a := NewSLMLocalAnalyzer()
	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show orders that are not shipped but shipped"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if !res.ClarificationNeeded {
		t.Fatalf("expected clarification for conflicting shipped cues")
	}
}

func TestSLMLocalNotShippedUsesNegationFilter(t *testing.T) {
	a := NewSLMLocalAnalyzer()
	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show orders that are not shipped"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.IntentCategory != "wismo" {
		t.Fatalf("expected wismo intent, got %s", res.IntentCategory)
	}
	if res.IntentSubcategory != "shipping_tracking" {
		t.Fatalf("expected shipping_tracking subcategory, got %s", res.IntentSubcategory)
	}
	foundNotShipped := false
	for _, f := range res.Query.Filters {
		if f.Field == "shipment.state" && f.Op == "neq" && f.Value == "Shipped" {
			foundNotShipped = true
			break
		}
	}
	if !foundNotShipped {
		t.Fatalf("expected shipment_state != Shipped filter, got %+v", res.Query.Filters)
	}
}

func TestSLMLocalHaveNotBeenShippedUsesNegationFilter(t *testing.T) {
	a := NewSLMLocalAnalyzer()
	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show me orders that have not been shipped"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	foundNotShipped := false
	for _, f := range res.Query.Filters {
		if f.Field == "shipment.state" && f.Op == "neq" && f.Value == "Shipped" {
			foundNotShipped = true
			break
		}
	}
	if !foundNotShipped {
		t.Fatalf("expected shipment_state != Shipped filter, got %+v", res.Query.Filters)
	}
}

func TestSLMLocalRemoteInvalidFallsBack(t *testing.T) {
	var calls int
	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			body := map[string]interface{}{
				"response": `{"intent":"search_order","intentCategory":"wismo","resourceType":"order","confidence":0.9,"clarificationNeeded":false,"safeEvidence":["remote"],"query":{"contractVersion":"v2","intentCategory":"wismo","filters":[{"field":"drop_table","op":"hack","value":"x"}],"sort":{"field":"order.created_at","dir":"desc"},"page":{"limit":20}}}`,
			}
			raw, _ := json.Marshal(body)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(string(raw))),
				Header:     make(http.Header),
			}, nil
		}),
	}
	a := &SLMLocalAnalyzer{endpoint: "http://mock", model: "test-model", httpClient: client}

	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show open orders this week"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if !strings.Contains(strings.Join(res.Notes, ","), "slm_remote_invalid_fallback") {
		t.Fatalf("expected fallback note, got notes=%v", res.Notes)
	}
	if len(res.ValidationErrors) == 0 {
		t.Fatalf("expected validation errors from invalid remote output")
	}
	if res.FinalValidatedQuery == nil {
		t.Fatalf("expected final validated query")
	}
	if calls < 2 {
		t.Fatalf("expected extract + repair attempts, got %d calls", calls)
	}
}

func TestSLMLocalRemoteRepairSucceeds(t *testing.T) {
	var calls int
	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			respPayload := map[string]interface{}{}
			if calls == 1 {
				respPayload["response"] = `{"intent":"search_order","intentCategory":"wismo","resourceType":"order","confidence":0.8,"clarificationNeeded":false,"safeEvidence":["remote_invalid"],"query":{"contractVersion":"v2","intentCategory":"wismo","filters":[{"field":"drop_table","op":"hack","value":"x"}],"sort":{"field":"order.created_at","dir":"desc"},"page":{"limit":20}}}`
			} else {
				respPayload["response"] = `{"intent":"search_order","intentCategory":"crm_profile","resourceType":"order","confidence":0.87,"clarificationNeeded":false,"safeEvidence":["remote_repaired"],"query":{"contractVersion":"v2","intentCategory":"crm_profile","filters":[{"field":"order.customer_email","op":"eq","value":"aster@example.com"}],"sort":{"field":"order.created_at","dir":"desc"},"page":{"limit":20}}}`
			}
			raw, _ := json.Marshal(respPayload)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(string(raw))),
				Header:     make(http.Header),
			}, nil
		}),
	}
	a := &SLMLocalAnalyzer{endpoint: "http://mock", model: "test-model", httpClient: client}

	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show orders for aster@example.com"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if !res.Repaired {
		t.Fatalf("expected repaired=true")
	}
	if res.IntentCategory != "crm_profile" {
		t.Fatalf("expected crm_profile after repair, got %s", res.IntentCategory)
	}
	if len(res.ValidationErrors) == 0 {
		t.Fatalf("expected initial validation errors to be recorded")
	}
	foundEmail := false
	for _, f := range res.Query.Filters {
		if f.Field == "order.customer_email" && f.Op == "eq" {
			foundEmail = true
			break
		}
	}
	if !foundEmail {
		t.Fatalf("expected repaired query to include customer_email filter")
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls (extract+repair), got %d", calls)
	}
}

func TestSLMLocalRemoteNotShippedGuardrailOverridesConflictingShippedFilter(t *testing.T) {
	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body := map[string]interface{}{
				"response": `{"intent":"search_order","intentCategory":"wismo","resourceType":"order","confidence":0.9,"clarificationNeeded":false,"safeEvidence":["remote"],"query":{"contractVersion":"v2","intentCategory":"wismo","filters":[{"field":"shipment.state","op":"eq","value":"Shipped"}],"sort":{"field":"order.created_at","dir":"desc"},"page":{"limit":20}}}`,
			}
			raw, _ := json.Marshal(body)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(string(raw))),
				Header:     make(http.Header),
			}, nil
		}),
	}
	a := &SLMLocalAnalyzer{endpoint: "http://mock", model: "test-model", httpClient: client}

	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show orders that have not been shipped"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	hasEqShipped := false
	hasNeqShipped := false
	hasNeqDelivered := false
	hasNeqReady := false
	for _, f := range res.Query.Filters {
		if f.Field == "shipment.state" && strings.EqualFold(f.Op, "eq") && f.Value == "Shipped" {
			hasEqShipped = true
		}
		if f.Field == "shipment.state" && strings.EqualFold(f.Op, "neq") && f.Value == "Shipped" {
			hasNeqShipped = true
		}
		if f.Field == "shipment.state" && strings.EqualFold(f.Op, "neq") && f.Value == "Delivered" {
			hasNeqDelivered = true
		}
		if f.Field == "shipment.state" && strings.EqualFold(f.Op, "neq") && f.Value == "Ready" {
			hasNeqReady = true
		}
	}
	if hasEqShipped {
		t.Fatalf("did not expect shipment.state eq Shipped after guardrail")
	}
	if !hasNeqShipped {
		t.Fatalf("expected shipment.state neq Shipped after guardrail")
	}
	if !hasNeqDelivered || !hasNeqReady {
		t.Fatalf("expected strict not-shipped exclusions (Delivered, Ready), got filters=%+v", res.Query.Filters)
	}
	if !strings.Contains(strings.Join(res.Notes, ","), "guardrail_not_shipped_enforced") {
		t.Fatalf("expected guardrail note, got %v", res.Notes)
	}
}

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRouterUsesOverrideProvider(t *testing.T) {
	router := NewRouter("rule-slm", map[string]Analyzer{
		"rule-slm":         NewRuleSLMAnalyzer(),
		"superlinked-mock": NewSuperlinkedMockAnalyzer(),
	})
	res, err := router.Analyze(context.Background(), AnalyzeRequest{
		Message:          "Check refund status for ORD-001",
		ProviderOverride: "superlinked-mock",
	})
	if err != nil {
		t.Fatalf("router analyze failed: %v", err)
	}
	if res.Provider != "superlinked-mock" {
		t.Fatalf("expected superlinked-mock provider, got %s", res.Provider)
	}
	if res.IntentCategory != "returns_refunds" {
		t.Fatalf("expected returns_refunds, got %s", res.IntentCategory)
	}
}

type staticAnalyzer struct {
	name string
	res  AnalyzeResult
	err  error
}

func (s staticAnalyzer) Name() string { return s.name }
func (s staticAnalyzer) Analyze(_ context.Context, _ AnalyzeRequest) (AnalyzeResult, error) {
	return s.res, s.err
}

type staticRetriever struct {
	ids []string
}

func (s staticRetriever) TopK(_ context.Context, _ string, _ string, _ string, _ int) ([]string, []string, error) {
	return s.ids, []string{"semantic_similarity:stub"}, nil
}

func TestSuperlinkedMockUsesRankOnlyForExplicitOperationalFilters(t *testing.T) {
	a := NewSuperlinkedMockAnalyzerWithFramerAndRetriever(
		NewDeterministicIntentFramer(),
		staticRetriever{ids: []string{"ord-00010", "ord-00042"}},
		10,
	)
	res, err := a.Analyze(context.Background(), AnalyzeRequest{
		Message:         "show open orders this week",
		ContractVersion: "v2",
		TenantID:        "tenant-a",
	})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	foundIDIn := false
	for _, f := range res.Query.Filters {
		if f.Field == "id" && strings.ToLower(f.Op) == "in" {
			foundIDIn = true
			break
		}
	}
	if foundIDIn {
		t.Fatalf("did not expect semantic top-k id in filter for explicit operational filters")
	}
	if !strings.Contains(strings.Join(res.Notes, ","), "semantic_vector_rank_only_due_to_explicit_filters") {
		t.Fatalf("expected rank-only note, got %v", res.Notes)
	}
}

func TestSuperlinkedMockUsesRankOnlyForExplicitCustomerFilters(t *testing.T) {
	a := NewSuperlinkedMockAnalyzerWithFramerAndRetriever(
		NewDeterministicIntentFramer(),
		staticRetriever{ids: []string{"cust-00010", "cust-00042"}},
		10,
	)
	res, err := a.Analyze(context.Background(), AnalyzeRequest{
		Message:         "show customer profile for customer00042@tenant-a.example.com",
		ContractVersion: "v2",
		TenantID:        "tenant-a",
		ResourceHint:    "customer",
	})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.ResourceType != "customer" {
		t.Fatalf("expected customer resource type, got %s", res.ResourceType)
	}
	foundIDIn := false
	for _, f := range res.Query.Filters {
		if f.Field == "id" && strings.ToLower(f.Op) == "in" {
			foundIDIn = true
			break
		}
	}
	if foundIDIn {
		t.Fatalf("did not expect semantic top-k id in filter for explicit customer filters")
	}
	if !strings.Contains(strings.Join(res.Notes, ","), "semantic_vector_rank_only_due_to_explicit_filters") {
		t.Fatalf("expected rank-only note, got %v", res.Notes)
	}
}

func TestSuperlinkedMockAppliesSemanticTopKIDFilterWhenNoExplicitFilters(t *testing.T) {
	a := NewSuperlinkedMockAnalyzerWithFramerAndRetriever(
		stubFramer{res: ParseResult{
			Query: store.QueryDSL{
				ContractVersion: "v2",
				IntentCategory:  "wismo",
				Filters:         []store.Filter{{Field: "order.created_at", Op: "gte", Value: "2026-01-01T00:00:00Z"}},
				Sort:            store.Sort{Field: "order.created_at", Dir: "desc"},
				Page:            store.Page{Limit: 20},
			},
			Intent:         "search_order",
			IntentCategory: "wismo",
			ResourceType:   "order",
			Confidence:     0.75,
		}},
		staticRetriever{ids: []string{"ord-00010", "ord-00042"}},
		10,
	)
	res, err := a.Analyze(context.Background(), AnalyzeRequest{
		Message:         "orders this week",
		ContractVersion: "v2",
		TenantID:        "tenant-a",
	})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	foundIDIn := false
	for _, f := range res.Query.Filters {
		if f.Field == "id" && strings.ToLower(f.Op) == "in" {
			foundIDIn = true
			break
		}
	}
	if !foundIDIn {
		t.Fatalf("expected semantic top-k id in filter when only temporal scope filter exists")
	}
}

func TestSLMSuperlinkedCombinedProvider(t *testing.T) {
	slm := staticAnalyzer{
		name: "slm-local",
		res: AnalyzeResult{
			Query: store.QueryDSL{
				ContractVersion: "v2",
				IntentCategory:  "wismo",
				Filters: []store.Filter{
					{Field: "shipment.state", Op: "eq", Value: "Delayed"},
				},
				Sort: store.Sort{Field: "order.created_at", Dir: "desc"},
				Page: store.Page{Limit: 20},
			},
			Intent:         "search_order",
			IntentCategory: "wismo",
			ResourceType:   "order",
			Confidence:     0.7,
			Provider:       "slm-local",
			SafeEvidence:   []string{"slm_intent"},
		},
	}
	superlinked := staticAnalyzer{
		name: "superlinked",
		res: AnalyzeResult{
			Query: store.QueryDSL{
				ContractVersion: "v2",
				IntentCategory:  "wismo",
				Filters: []store.Filter{
					{Field: "shipment.tracking_id", Op: "eq", Value: "TRK-123"},
				},
				Sort: store.Sort{Field: "order.created_at", Dir: "desc"},
				Page: store.Page{Limit: 10},
			},
			Intent:         "search_order",
			IntentCategory: "wismo",
			ResourceType:   "order",
			Confidence:     0.9,
			Provider:       "superlinked",
			SafeEvidence:   []string{"superlinked_similarity"},
		},
	}

	combined := NewSLMSuperlinkedAnalyzer(slm, superlinked)
	res, err := combined.Analyze(context.Background(), AnalyzeRequest{Message: "where is my order"})
	if err != nil {
		t.Fatalf("combined analyze failed: %v", err)
	}
	if res.Provider != "slm-superlinked" {
		t.Fatalf("expected slm-superlinked provider, got %s", res.Provider)
	}
	if len(res.Query.Filters) != 2 {
		t.Fatalf("expected merged filters from slm + superlinked, got %d", len(res.Query.Filters))
	}
	if len(res.FilterSource) != 2 {
		t.Fatalf("expected filterSource entries, got %d", len(res.FilterSource))
	}
	sources := map[string]struct{}{}
	for _, fs := range res.FilterSource {
		sources[fs.Source] = struct{}{}
	}
	if _, ok := sources["slm-local"]; !ok {
		t.Fatalf("expected slm-local filter source")
	}
	if _, ok := sources["superlinked"]; !ok {
		t.Fatalf("expected superlinked filter source")
	}
}
