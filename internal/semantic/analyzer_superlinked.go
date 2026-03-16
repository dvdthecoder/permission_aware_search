package semantic

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"permission_aware_search/internal/contracts"
	"permission_aware_search/internal/store"
)

type SuperlinkedAnalyzer struct {
	fallback Analyzer
	framer   IntentFramer
	gateway  SuperlinkedGateway
	gate     ServingGate
	topK     int
	postCap  int
}

func NewSuperlinkedAnalyzer(endpoint string, timeout time.Duration, fallback Analyzer) *SuperlinkedAnalyzer {
	return NewSuperlinkedAnalyzerWithFramer(endpoint, timeout, fallback, NewDeterministicIntentFramer())
}

func NewSuperlinkedAnalyzerWithFramer(endpoint string, timeout time.Duration, fallback Analyzer, framer IntentFramer) *SuperlinkedAnalyzer {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if framer == nil {
		framer = NewDeterministicIntentFramer()
	}
	topK := intFromEnvOr("SUPERLINKED_TOPK", 100)
	postCap := intFromEnvOr("SUPERLINKED_POST_MERGE_CAP", 300)
	return &SuperlinkedAnalyzer{
		fallback: fallback,
		framer:   framer,
		gateway:  NewHTTPSuperlinkedGateway(endpoint, timeout, topK),
		topK:     topK,
		postCap:  postCap,
		gate: ServingGate{
			Mode:          strings.ToLower(strings.TrimSpace(envOrDefault("SUPERLINKED_MODE", "shadow"))),
			MinConfidence: floatFromEnvOr("SUPERLINKED_MIN_CONFIDENCE", 0.55),
			MaxLatencyMs:  int64(intFromEnvOr("SUPERLINKED_MAX_LATENCY_MS", 120)),
			MaxCandidates: postCap,
		},
	}
}

func (a *SuperlinkedAnalyzer) Name() string { return "superlinked" }

func (a *SuperlinkedAnalyzer) Analyze(ctx context.Context, req AnalyzeRequest) (AnalyzeResult, error) {
	if req.ContractVersion == "" {
		req.ContractVersion = contracts.ContractVersionV2
	}
	if a.gateway == nil {
		return a.fallbackWithNote(ctx, req, "superlinked endpoint not configured")
	}

	heuristic := a.framer.Frame(req.Message, req.ContractVersion, req.ResourceHint)
	provider, err := a.gateway.Analyze(ctx, GatewayRequest{
		TenantID:        req.TenantID,
		Message:         req.Message,
		ContractVersion: req.ContractVersion,
		ResourceHint:    req.ResourceHint,
		IntentCategory:  heuristic.IntentCategory,
		TopK:            a.topK,
	})
	if err != nil {
		return a.fallbackWithNote(ctx, req, fmt.Sprintf("superlinked gateway error: %v", err))
	}

	decision := a.gate.Decide(provider.ProviderConfidence, provider.ProviderLatencyMs, len(provider.CandidateIDs), heuristic.ClarificationNeeded)
	if !decision.ServeSuperlinked {
		return a.fallbackWithNote(ctx, req, "superlinked_gated_fallback:"+decision.Reason)
	}

	query := heuristic.Query
	query.ContractVersion = req.ContractVersion
	if query.IntentCategory == "" {
		query.IntentCategory = heuristic.IntentCategory
	}
	if hasAuthoritativeSupportFilter(heuristic.ResourceType, query.Filters) {
		pre := heuristic.Query
		post := query
		return AnalyzeResult{
			Query:                  query,
			OriginalMessage:        req.Message,
			RewrittenMessage:       req.Message,
			NormalizedInput:        heuristic.NormalizedInput,
			NormalizationApplied:   heuristic.NormalizationApplied,
			ExtractedSlots:         &heuristic.ExtractedSlots,
			PreSemanticQuery:       &pre,
			PostSemanticQuery:      &post,
			Intent:                 heuristic.Intent,
			IntentCategory:         heuristic.IntentCategory,
			IntentSubcategory:      heuristic.IntentSubcategory,
			ResourceType:           heuristic.ResourceType,
			Confidence:             maxFloat(provider.ProviderConfidence, heuristic.Confidence),
			ClarificationNeeded:    false,
			Provider:               a.Name(),
			Notes:                  []string{"superlinked_gateway_ok", "superlinked_served", "superlinked_rank_only_due_to_explicit_filters"},
			SafeEvidence:           appendUnique(heuristic.SafeEvidence, provider.SafeEvidence...),
			FilterSource:           sourceForFilters(query.Filters, a.Name()),
			RetrievalProvider:      "superlinked_gateway",
			RetrievalModelVersion:  provider.ModelVersion,
			RetrievalFallbackChain: []string{"superlinked_gateway", "deterministic_query_only"},
			RetrievalGateReason:    "",
			RetrievalStrategy:      "hybrid_fusion",
			RetrievalLatencyMs:     provider.ProviderLatencyMs,
			RetrievalEvidence:      appendUnique(provider.SafeEvidence, "retrieval_rank_only"),
		}, nil
	}

	inVals := make([]interface{}, 0, len(provider.CandidateIDs))
	for i, id := range provider.CandidateIDs {
		if a.postCap > 0 && i >= a.postCap {
			break
		}
		inVals = append(inVals, id)
	}
	if len(inVals) == 0 {
		return a.fallbackWithNote(ctx, req, "superlinked_gated_fallback:no_candidates")
	}
	query.Filters = append(query.Filters, store.Filter{Field: "id", Op: "in", Value: inVals})

	pre := heuristic.Query
	post := query

	return AnalyzeResult{
		Query:                  query,
		OriginalMessage:        req.Message,
		RewrittenMessage:       req.Message,
		NormalizedInput:        heuristic.NormalizedInput,
		NormalizationApplied:   heuristic.NormalizationApplied,
		ExtractedSlots:         &heuristic.ExtractedSlots,
		PreSemanticQuery:       &pre,
		PostSemanticQuery:      &post,
		Intent:                 heuristic.Intent,
		IntentCategory:         heuristic.IntentCategory,
		IntentSubcategory:      heuristic.IntentSubcategory,
		ResourceType:           heuristic.ResourceType,
		Confidence:             maxFloat(provider.ProviderConfidence, heuristic.Confidence),
		ClarificationNeeded:    false,
		Provider:               a.Name(),
		Notes:                  []string{"superlinked_gateway_ok", "superlinked_served"},
		SafeEvidence:           appendUnique(heuristic.SafeEvidence, provider.SafeEvidence...),
		FilterSource:           sourceForFilters(query.Filters, a.Name()),
		RetrievalProvider:      "superlinked_gateway",
		RetrievalModelVersion:  provider.ModelVersion,
		RetrievalFallbackChain: []string{"superlinked_gateway", "deterministic_query_only"},
		RetrievalGateReason:    "",
		RetrievalStrategy:      "vector_only_fallback",
		RetrievalLatencyMs:     provider.ProviderLatencyMs,
		RetrievalEvidence:      provider.SafeEvidence,
	}, nil
}

func (a *SuperlinkedAnalyzer) fallbackWithNote(ctx context.Context, req AnalyzeRequest, note string) (AnalyzeResult, error) {
	fallbackResult, err := a.fallback.Analyze(ctx, req)
	if err != nil {
		return AnalyzeResult{}, err
	}
	fallbackResult.Provider = a.Name() + "(fallback:" + a.fallback.Name() + ")"
	fallbackResult.Notes = append(fallbackResult.Notes, note)
	return fallbackResult, nil
}

func intFromEnvOr(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func floatFromEnvOr(name string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
