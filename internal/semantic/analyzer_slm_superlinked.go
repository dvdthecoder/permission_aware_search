package semantic

import (
	"context"

	"permission_aware_search/internal/store"
)

// SLMSuperlinkedAnalyzer combines local SLM intent/query rewrite with
// superlinked semantic refinement in a single provider mode.
type SLMSuperlinkedAnalyzer struct {
	slm         Analyzer
	superlinked Analyzer
}

func NewSLMSuperlinkedAnalyzer(slm Analyzer, superlinked Analyzer) *SLMSuperlinkedAnalyzer {
	return &SLMSuperlinkedAnalyzer{slm: slm, superlinked: superlinked}
}

func (a *SLMSuperlinkedAnalyzer) Name() string { return "slm-superlinked" }

func (a *SLMSuperlinkedAnalyzer) Analyze(ctx context.Context, req AnalyzeRequest) (AnalyzeResult, error) {
	slmRes, err := a.slm.Analyze(ctx, req)
	if err != nil {
		return AnalyzeResult{}, err
	}

	refineReq := req
	if refineReq.ContractVersion == "" {
		refineReq.ContractVersion = slmRes.Query.ContractVersion
	}
	if refineReq.ResourceHint == "" {
		refineReq.ResourceHint = slmRes.ResourceType
	}

	supRes, err := a.superlinked.Analyze(ctx, refineReq)
	if err != nil {
		fallback := slmRes
		fallback.Provider = a.Name() + "(fallback:slm-local)"
		fallback.Notes = appendUnique(fallback.Notes, "superlinked_refinement_failed")
		fallback.FilterSource = sourceForFilters(fallback.Query.Filters, "slm-local")
		pre := slmRes.Query
		post := fallback.Query
		fallback.PreSemanticQuery = &pre
		fallback.PostSemanticQuery = &post
		return fallback, nil
	}

	merged := supRes
	merged.Provider = a.Name()
	if merged.Intent == "" {
		merged.Intent = slmRes.Intent
	}
	if merged.IntentCategory == "" {
		merged.IntentCategory = slmRes.IntentCategory
	}
	if merged.IntentSubcategory == "" {
		merged.IntentSubcategory = slmRes.IntentSubcategory
	}
	if merged.ResourceType == "" {
		merged.ResourceType = slmRes.ResourceType
	}
	if merged.Confidence < slmRes.Confidence {
		merged.Confidence = slmRes.Confidence
	}
	merged.Query = mergeQueryDSL(slmRes.Query, supRes.Query)
	if merged.Query.IntentCategory == "" {
		merged.Query.IntentCategory = merged.IntentCategory
	}
	merged.Notes = appendUnique(appendUnique(slmRes.Notes, supRes.Notes...), "slm_rewrite_plus_superlinked_refinement")
	merged.SafeEvidence = appendUnique(slmRes.SafeEvidence, supRes.SafeEvidence...)
	if merged.OriginalMessage == "" {
		if slmRes.OriginalMessage != "" {
			merged.OriginalMessage = slmRes.OriginalMessage
		} else {
			merged.OriginalMessage = req.Message
		}
	}
	if merged.RewrittenMessage == "" {
		if slmRes.RewrittenMessage != "" {
			merged.RewrittenMessage = slmRes.RewrittenMessage
		} else {
			merged.RewrittenMessage = req.Message
		}
	}
	if merged.NormalizedInput == "" {
		merged.NormalizedInput = slmRes.NormalizedInput
	}
	if len(merged.NormalizationApplied) == 0 {
		merged.NormalizationApplied = append([]string{}, slmRes.NormalizationApplied...)
	}
	if merged.ExtractedSlots == nil && slmRes.ExtractedSlots != nil {
		copied := *slmRes.ExtractedSlots
		merged.ExtractedSlots = &copied
	}
	pre := slmRes.Query
	post := merged.Query
	merged.PreSemanticQuery = &pre
	merged.PostSemanticQuery = &post
	merged.FilterSource = mergeFilterSources(
		merged.Query.Filters,
		sourceForFilters(slmRes.Query.Filters, "slm-local"),
		sourceForFilters(supRes.Query.Filters, "superlinked"),
	)
	if merged.RetrievalProvider == "" {
		merged.RetrievalProvider = supRes.RetrievalProvider
	}
	if merged.RetrievalModelVersion == "" {
		merged.RetrievalModelVersion = supRes.RetrievalModelVersion
	}
	if len(merged.RetrievalFallbackChain) == 0 {
		merged.RetrievalFallbackChain = append([]string{}, supRes.RetrievalFallbackChain...)
	}
	if merged.RetrievalGateReason == "" {
		merged.RetrievalGateReason = supRes.RetrievalGateReason
	}
	if merged.RetrievalStrategy == "" {
		merged.RetrievalStrategy = supRes.RetrievalStrategy
	}
	if merged.RetrievalLatencyMs == 0 {
		merged.RetrievalLatencyMs = supRes.RetrievalLatencyMs
	}
	merged.RetrievalEvidence = appendUnique(supRes.RetrievalEvidence, merged.RetrievalEvidence...)
	merged.RetrievalScores = appendUnique(supRes.RetrievalScores, merged.RetrievalScores...)
	return merged, nil
}

func mergeQueryDSL(primary, secondary store.QueryDSL) store.QueryDSL {
	out := primary
	if secondary.ContractVersion != "" {
		out.ContractVersion = secondary.ContractVersion
	}
	if secondary.IntentCategory != "" {
		out.IntentCategory = secondary.IntentCategory
	}
	if secondary.Sort.Field != "" {
		out.Sort = secondary.Sort
	}
	if secondary.Page.Limit > 0 {
		out.Page = secondary.Page
	}
	out.Filters = mergeFilters(primary.Filters, secondary.Filters)
	return out
}

func mergeFilters(a, b []store.Filter) []store.Filter {
	out := make([]store.Filter, 0, len(a)+len(b))
	seen := map[string]struct{}{}
	for _, f := range append(a, b...) {
		key := filterSourceKey(f.Field, f.Op, f.Value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, f)
	}
	return out
}

func appendUnique(base []string, extra ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(base)+len(extra))
	for _, v := range base {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	for _, v := range extra {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
