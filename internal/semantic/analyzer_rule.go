package semantic

import (
	"context"

	"permission_aware_search/internal/contracts"
)

type RuleSLMAnalyzer struct {
	framer IntentFramer
}

func NewRuleSLMAnalyzer() *RuleSLMAnalyzer {
	return NewRuleSLMAnalyzerWithFramer(NewDeterministicIntentFramer())
}

func NewRuleSLMAnalyzerWithFramer(framer IntentFramer) *RuleSLMAnalyzer {
	if framer == nil {
		framer = NewDeterministicIntentFramer()
	}
	return &RuleSLMAnalyzer{framer: framer}
}

func (a *RuleSLMAnalyzer) Name() string { return "rule-slm" }

func (a *RuleSLMAnalyzer) Analyze(_ context.Context, req AnalyzeRequest) (AnalyzeResult, error) {
	if req.ContractVersion == "" {
		req.ContractVersion = contracts.ContractVersionV2
	}
	parsed := a.framer.Frame(req.Message, req.ContractVersion, req.ResourceHint)
	pre := parsed.Query
	post := parsed.Query

	return AnalyzeResult{
		Query:                parsed.Query,
		OriginalMessage:      req.Message,
		RewrittenMessage:     req.Message,
		NormalizedInput:      parsed.NormalizedInput,
		NormalizationApplied: parsed.NormalizationApplied,
		ExtractedSlots:       &parsed.ExtractedSlots,
		PreSemanticQuery:     &pre,
		PostSemanticQuery:    &post,
		Intent:               parsed.Intent,
		IntentCategory:       parsed.IntentCategory,
		IntentSubcategory:    parsed.IntentSubcategory,
		ResourceType:         parsed.ResourceType,
		Confidence:           parsed.Confidence,
		ClarificationNeeded:  parsed.ClarificationNeeded,
		Provider:             a.Name(),
		SafeEvidence:         parsed.SafeEvidence,
		FilterSource:         sourceForFilters(parsed.Query.Filters, a.Name()),
	}, nil
}
