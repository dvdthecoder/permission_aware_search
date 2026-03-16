package semantic

import (
	"context"

	"permission_aware_search/internal/contracts"
	"permission_aware_search/internal/store"
)

// SuperlinkedMockAnalyzer is a local deterministic/hybrid semantic stub.
// It returns production-shape semantic outputs without external dependencies.
type SuperlinkedMockAnalyzer struct {
	framer    IntentFramer
	retriever SemanticCandidateRetriever
	topK      int
}

func NewSuperlinkedMockAnalyzer() *SuperlinkedMockAnalyzer {
	return NewSuperlinkedMockAnalyzerWithFramerAndRetriever(NewDeterministicIntentFramer(), nil, 0)
}

func NewSuperlinkedMockAnalyzerWithFramer(framer IntentFramer) *SuperlinkedMockAnalyzer {
	return NewSuperlinkedMockAnalyzerWithFramerAndRetriever(framer, nil, 0)
}

func NewSuperlinkedMockAnalyzerWithFramerAndRetriever(framer IntentFramer, retriever SemanticCandidateRetriever, topK int) *SuperlinkedMockAnalyzer {
	if framer == nil {
		framer = NewDeterministicIntentFramer()
	}
	if topK <= 0 {
		topK = 25
	}
	return &SuperlinkedMockAnalyzer{framer: framer, retriever: retriever, topK: topK}
}

func (a *SuperlinkedMockAnalyzer) Name() string { return "superlinked-mock" }

func (a *SuperlinkedMockAnalyzer) Analyze(ctx context.Context, req AnalyzeRequest) (AnalyzeResult, error) {
	if req.ContractVersion == "" {
		req.ContractVersion = contracts.ContractVersionV2
	}
	parsed := a.framer.Frame(req.Message, req.ContractVersion, req.ResourceHint)
	pre := parsed.Query
	query := parsed.Query

	notes := []string{"hybrid_semantic_stub", "safe_projection_only"}
	safeEvidence := append([]string{}, parsed.SafeEvidence...)
	retrievalProvider := ""
	retrievalModel := ""
	retrievalChain := []string{}
	retrievalGateReason := ""
	retrievalStrategy := ""
	retrievalLatency := int64(0)
	retrievalScores := []string{}
	if !parsed.ClarificationNeeded && req.TenantID != "" && (parsed.ResourceType == "order" || parsed.ResourceType == "customer") && a.retriever != nil {
		ids := []string{}
		evidence := []string{}
		if detailed, ok := a.retriever.(DetailedSemanticCandidateRetriever); ok {
			res, err := detailed.Retrieve(ctx, RetrievalRequest{
				TenantID:       req.TenantID,
				ResourceType:   parsed.ResourceType,
				Message:        req.Message,
				QueryShape:     req.QueryShape,
				IntentCategory: parsed.IntentCategory,
				TopK:           a.topK,
			})
			if err != nil {
				notes = append(notes, "semantic_candidate_retrieval_failed")
			} else {
				ids = res.CandidateIDs
				evidence = res.Evidence
				retrievalProvider = res.Provider
				retrievalModel = res.ModelVersion
				retrievalChain = res.FallbackChain
				retrievalGateReason = res.GateReason
				retrievalStrategy = res.Strategy
				retrievalLatency = res.LatencyMs
				retrievalScores = res.ScoredEvidence
			}
		} else {
			var err error
			ids, evidence, err = a.retriever.TopK(ctx, req.TenantID, parsed.ResourceType, req.Message, a.topK)
			if err != nil {
				notes = append(notes, "semantic_candidate_retrieval_failed")
			}
		}
		if len(ids) > 0 {
			if hasAuthoritativeSupportFilter(parsed.ResourceType, query.Filters) {
				notes = append(notes, "semantic_vector_rank_only_due_to_explicit_filters")
				safeEvidence = append(safeEvidence, "retrieval_rank_only")
				safeEvidence = append(safeEvidence, evidence...)
			} else {
				inVals := make([]interface{}, 0, len(ids))
				for _, id := range ids {
					inVals = append(inVals, id)
				}
				query.Filters = append(query.Filters, store.Filter{Field: "id", Op: "in", Value: inVals})
				notes = append(notes, "semantic_vector_topk_applied")
				safeEvidence = append(safeEvidence, evidence...)
			}
		}
	}
	post := query

	confidence := parsed.Confidence + 0.08
	if confidence > 0.99 {
		confidence = 0.99
	}

	return AnalyzeResult{
		Query:                  query,
		OriginalMessage:        req.Message,
		RewrittenMessage:       req.Message,
		NormalizedInput:        parsed.NormalizedInput,
		NormalizationApplied:   parsed.NormalizationApplied,
		ExtractedSlots:         &parsed.ExtractedSlots,
		PreSemanticQuery:       &pre,
		PostSemanticQuery:      &post,
		Intent:                 parsed.Intent,
		IntentCategory:         parsed.IntentCategory,
		IntentSubcategory:      parsed.IntentSubcategory,
		ResourceType:           parsed.ResourceType,
		Confidence:             confidence,
		ClarificationNeeded:    parsed.ClarificationNeeded,
		Provider:               a.Name(),
		Notes:                  notes,
		SafeEvidence:           safeEvidence,
		FilterSource:           sourceForFilters(query.Filters, a.Name()),
		RetrievalProvider:      retrievalProvider,
		RetrievalModelVersion:  retrievalModel,
		RetrievalFallbackChain: retrievalChain,
		RetrievalGateReason:    retrievalGateReason,
		RetrievalStrategy:      retrievalStrategy,
		RetrievalLatencyMs:     retrievalLatency,
		RetrievalEvidence:      safeEvidence,
		RetrievalScores:        retrievalScores,
	}, nil
}
