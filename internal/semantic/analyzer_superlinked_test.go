package semantic

import (
	"context"
	"strings"
	"testing"

	"permission_aware_search/internal/store"
)

type stubGateway struct {
	resp GatewayResponse
	err  error
}

func (s stubGateway) Analyze(_ context.Context, _ GatewayRequest) (GatewayResponse, error) {
	return s.resp, s.err
}

type stubFramer struct {
	res ParseResult
}

func (s stubFramer) Name() string                     { return "stub-framer" }
func (s stubFramer) Normalize(message string) string  { return message }
func (s stubFramer) Frame(_, _, _ string) ParseResult { return s.res }

func TestSuperlinkedAnalyzerServedUsesRankOnlyWhenExplicitFiltersPresent(t *testing.T) {
	a := &SuperlinkedAnalyzer{
		fallback: staticAnalyzer{
			name: "slm-local",
			res:  AnalyzeResult{Query: store.QueryDSL{ContractVersion: "v2"}, Provider: "slm-local"},
		},
		framer:  NewDeterministicIntentFramer(),
		gateway: stubGateway{resp: GatewayResponse{CandidateIDs: []string{"ord-1", "ord-2"}, ProviderConfidence: 0.9, ProviderLatencyMs: 20}},
		gate:    ServingGate{Mode: "gated", MinConfidence: 0.55, MaxLatencyMs: 120, MaxCandidates: 300},
		topK:    100,
		postCap: 300,
	}

	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show open orders this week", ContractVersion: "v2", TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	foundIDIn := false
	for _, f := range res.Query.Filters {
		if f.Field == "id" && strings.ToLower(f.Op) == "in" {
			foundIDIn = true
			break
		}
	}
	if foundIDIn {
		t.Fatalf("did not expect id in filter when explicit operational filters are present")
	}
	if !strings.Contains(strings.Join(res.Notes, ","), "superlinked_served") {
		t.Fatalf("expected superlinked_served note, got %v", res.Notes)
	}
	if !strings.Contains(strings.Join(res.Notes, ","), "superlinked_rank_only_due_to_explicit_filters") {
		t.Fatalf("expected rank-only note, got %v", res.Notes)
	}
}

func TestSuperlinkedAnalyzerServedAddsIDInFilterWithoutExplicitFilters(t *testing.T) {
	a := &SuperlinkedAnalyzer{
		fallback: staticAnalyzer{
			name: "slm-local",
			res:  AnalyzeResult{Query: store.QueryDSL{ContractVersion: "v2"}, Provider: "slm-local"},
		},
		framer: stubFramer{res: ParseResult{
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
		gateway: stubGateway{resp: GatewayResponse{CandidateIDs: []string{"ord-1", "ord-2"}, ProviderConfidence: 0.9, ProviderLatencyMs: 20}},
		gate:    ServingGate{Mode: "gated", MinConfidence: 0.55, MaxLatencyMs: 120, MaxCandidates: 300},
		topK:    100,
		postCap: 300,
	}

	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "orders this week", ContractVersion: "v2", TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	foundIDIn := false
	for _, f := range res.Query.Filters {
		if f.Field == "id" && strings.ToLower(f.Op) == "in" {
			foundIDIn = true
			break
		}
	}
	if !foundIDIn {
		t.Fatalf("expected id in filter when only temporal scope filters are present")
	}
}

func TestSuperlinkedAnalyzerGatedFallbackPreservesDeterministicQuery(t *testing.T) {
	fallbackQuery := store.QueryDSL{
		ContractVersion: "v2",
		Filters:         []store.Filter{{Field: "order.state", Op: "in", Value: []interface{}{"Open", "Confirmed"}}},
		Sort:            store.Sort{Field: "order.created_at", Dir: "desc"},
		Page:            store.Page{Limit: 20},
	}
	a := &SuperlinkedAnalyzer{
		fallback: staticAnalyzer{
			name: "slm-local",
			res: AnalyzeResult{
				Query:          fallbackQuery,
				Provider:       "slm-local",
				Intent:         "search_order",
				IntentCategory: "wismo",
				ResourceType:   "order",
			},
		},
		framer:  NewDeterministicIntentFramer(),
		gateway: stubGateway{resp: GatewayResponse{CandidateIDs: []string{"ord-1"}, ProviderConfidence: 0.1, ProviderLatencyMs: 20}},
		gate:    ServingGate{Mode: "gated", MinConfidence: 0.55, MaxLatencyMs: 120, MaxCandidates: 300},
	}

	res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: "show open orders this week", ContractVersion: "v2", TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if !strings.Contains(res.Provider, "fallback:slm-local") {
		t.Fatalf("expected fallback provider, got %s", res.Provider)
	}
	if len(res.Query.Filters) != len(fallbackQuery.Filters) {
		t.Fatalf("expected deterministic query preserved, got %v", res.Query.Filters)
	}
	joinedNotes := strings.Join(res.Notes, ",")
	if !strings.Contains(joinedNotes, "superlinked_gated_fallback:low_confidence") {
		t.Fatalf("expected low_confidence fallback note, got %v", res.Notes)
	}
}
