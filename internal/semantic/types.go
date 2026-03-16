package semantic

import (
	"context"

	"permission_aware_search/internal/store"
)

type AnalyzeRequest struct {
	Message          string
	ContractVersion  string
	ResourceHint     string
	TenantID         string
	QueryShape       string
	ProviderOverride string
}

type AnalyzeResult struct {
	Query                      store.QueryDSL         `json:"query"`
	OriginalMessage            string                 `json:"originalMessage,omitempty"`
	RewrittenMessage           string                 `json:"rewrittenMessage,omitempty"`
	NormalizedInput            string                 `json:"normalizedInput,omitempty"`
	NormalizationApplied       []string               `json:"normalizationApplied,omitempty"`
	ExtractedSlots             *SemanticSlots         `json:"extractedSlots,omitempty"`
	PreSemanticQuery           *store.QueryDSL        `json:"preSemanticQuery,omitempty"`
	PostSemanticQuery          *store.QueryDSL        `json:"postSemanticQuery,omitempty"`
	Intent                     string                 `json:"intent"`
	IntentCategory             string                 `json:"intentCategory"`
	IntentSubcategory          string                 `json:"intentSubcategory,omitempty"`
	ResourceType               string                 `json:"resourceType"`
	Confidence                 float64                `json:"confidence"`
	ClarificationNeeded        bool                   `json:"clarificationNeeded"`
	Provider                   string                 `json:"provider"`
	Notes                      []string               `json:"notes,omitempty"`
	SafeEvidence               []string               `json:"safeEvidence,omitempty"`
	FilterSource               []FilterSource         `json:"filterSource,omitempty"`
	SLMRaw                     map[string]interface{} `json:"slmRaw,omitempty"`
	ValidationErrors           []string               `json:"validationErrors,omitempty"`
	Repaired                   bool                   `json:"repaired,omitempty"`
	FinalValidatedQuery        *store.QueryDSL        `json:"finalValidatedQuery,omitempty"`
	RewriteIntentProvider      string                 `json:"rewriteIntentProvider,omitempty"`
	RewriteIntentModelVersion  string                 `json:"rewriteIntentModelVersion,omitempty"`
	RewriteIntentFallbackChain []string               `json:"rewriteIntentFallbackChain,omitempty"`
	RewriteIntentGateReason    string                 `json:"rewriteIntentGateReason,omitempty"`
	RetrievalProvider          string                 `json:"retrievalProvider,omitempty"`
	RetrievalModelVersion      string                 `json:"retrievalModelVersion,omitempty"`
	RetrievalFallbackChain     []string               `json:"retrievalFallbackChain,omitempty"`
	RetrievalGateReason        string                 `json:"retrievalGateReason,omitempty"`
	RetrievalStrategy          string                 `json:"retrievalStrategy,omitempty"`
	RetrievalLatencyMs         int64                  `json:"retrievalLatencyMs,omitempty"`
	RetrievalEvidence          []string               `json:"retrievalEvidence,omitempty"`
	RetrievalScores            []string               `json:"retrievalScores,omitempty"`
}

type FilterSource struct {
	Field  string      `json:"field"`
	Op     string      `json:"op"`
	Value  interface{} `json:"value"`
	Source string      `json:"source"`
}

type Analyzer interface {
	Name() string
	Analyze(ctx context.Context, req AnalyzeRequest) (AnalyzeResult, error)
}

type Router struct {
	defaultProvider string
	providers       map[string]Analyzer
}

func NewRouter(defaultProvider string, providers map[string]Analyzer) *Router {
	if defaultProvider == "" {
		defaultProvider = "rule-slm"
	}
	if _, ok := providers[defaultProvider]; !ok {
		defaultProvider = "rule-slm"
	}
	return &Router{defaultProvider: defaultProvider, providers: providers}
}

func (r *Router) Analyze(ctx context.Context, req AnalyzeRequest) (AnalyzeResult, error) {
	providerName := r.defaultProvider
	if req.ProviderOverride != "" {
		if _, ok := r.providers[req.ProviderOverride]; ok {
			providerName = req.ProviderOverride
		}
	}
	provider, ok := r.providers[providerName]
	if !ok {
		provider = r.providers["rule-slm"]
	}
	return provider.Analyze(ctx, req)
}
