package semantic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"permission_aware_search/internal/contracts"
	"permission_aware_search/internal/store"
)

// SLMLocalAnalyzer simulates an on-box small language model used for
// intent detection + query rewrite only.
type SLMLocalAnalyzer struct {
	endpoint      string
	model         string
	httpClient    *http.Client
	framer        IntentFramer
	promptBuilder *PromptBuilder
}

func NewSLMLocalAnalyzer() *SLMLocalAnalyzer {
	return NewSLMLocalAnalyzerWithConfig(envOrDefault("OLLAMA_ENDPOINT", ""), envOrDefault("OLLAMA_MODEL", "llama3.2:latest"), timeoutFromEnvMS("OLLAMA_TIMEOUT_MS", 1500), NewDeterministicIntentFramer())
}

func NewSLMLocalAnalyzerWithFramer(framer IntentFramer) *SLMLocalAnalyzer {
	return NewSLMLocalAnalyzerWithConfig(envOrDefault("OLLAMA_ENDPOINT", ""), envOrDefault("OLLAMA_MODEL", "llama3.2:latest"), timeoutFromEnvMS("OLLAMA_TIMEOUT_MS", 1500), framer)
}

func NewSLMLocalAnalyzerWithConfig(endpoint, model string, timeout time.Duration, framer IntentFramer) *SLMLocalAnalyzer {
	if framer == nil {
		framer = NewDeterministicIntentFramer()
	}
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}
	// Initialize prompt builder with default providers
	promptBuilder := NewPromptBuilder(
		GetDefaultSchemaProvider(),
		GetDefaultExampleProvider(),
	)
	return &SLMLocalAnalyzer{
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		framer:        framer,
		promptBuilder: promptBuilder,
	}
}

func timeoutFromEnvMS(name string, fallback int) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return time.Duration(fallback) * time.Millisecond
	}
	ms, err := time.ParseDuration(raw + "ms")
	if err != nil {
		return time.Duration(fallback) * time.Millisecond
	}
	return ms
}

func (a *SLMLocalAnalyzer) Name() string { return "slm-local" }

func (a *SLMLocalAnalyzer) Analyze(_ context.Context, req AnalyzeRequest) (AnalyzeResult, error) {
	if req.ContractVersion == "" {
		req.ContractVersion = contracts.ContractVersionV2
	}
	if a.framer == nil {
		a.framer = NewDeterministicIntentFramer()
	}

	rewritten := a.framer.Normalize(req.Message)
	parsedFallback := a.framer.Frame(rewritten, req.ContractVersion, req.ResourceHint)
	fallback := AnalyzeResult{
		Query:                parsedFallback.Query,
		OriginalMessage:      req.Message,
		RewrittenMessage:     rewritten,
		NormalizedInput:      parsedFallback.NormalizedInput,
		NormalizationApplied: parsedFallback.NormalizationApplied,
		ExtractedSlots:       &parsedFallback.ExtractedSlots,
		Intent:               parsedFallback.Intent,
		IntentCategory:       parsedFallback.IntentCategory,
		IntentSubcategory:    parsedFallback.IntentSubcategory,
		ResourceType:         parsedFallback.ResourceType,
		Confidence:           minFloat(parsedFallback.Confidence+0.05, 0.98),
		ClarificationNeeded:  parsedFallback.ClarificationNeeded,
		Provider:             a.Name(),
		Notes:                []string{"query_rewrite_applied", "slm_remote_fallback"},
		SafeEvidence:         parsedFallback.SafeEvidence,
	}
	applyMessageGuardrails(req.Message, &fallback)
	fallbackPre := fallback.Query
	fallbackPost := fallback.Query
	fallback.PreSemanticQuery = &fallbackPre
	fallback.PostSemanticQuery = &fallbackPost
	fallback.FinalValidatedQuery = &fallback.Query
	fallback.FilterSource = sourceForFilters(fallback.Query.Filters, a.Name())

	if remote, ok := a.tryRemoteSLM(req); ok {
		remote.Provider = a.Name()
		remote.OriginalMessage = req.Message
		remote.RewrittenMessage = rewritten
		remote.NormalizedInput = parsedFallback.NormalizedInput
		remote.NormalizationApplied = parsedFallback.NormalizationApplied
		remote.ExtractedSlots = &parsedFallback.ExtractedSlots
		remoteSLMRaw := asRawRemote(remote)

		normalized, errs := validateRemoteResult(remote, req.ContractVersion)
		validationErrors := append([]string{}, errs...)
		repaired := false

		if len(errs) > 0 {
			if repairedRes, okRepair := a.tryRepairRemoteSLM(req, remoteSLMRaw, errs); okRepair {
				repaired = true
				remoteSLMRaw = asRawRemote(repairedRes)
				normalized, errs = validateRemoteResult(repairedRes, req.ContractVersion)
				validationErrors = append(validationErrors, errs...)
				if len(errs) == 0 {
					remote = normalized
					remote.Notes = append(remote.Notes, "slm_remote_repair_applied")
				}
			}
		}

		if len(errs) > 0 {
			fallback.Notes = append(fallback.Notes, "slm_remote_invalid_fallback")
			fallback.SLMRaw = remoteSLMRaw
			fallback.ValidationErrors = dedupeStrings(validationErrors)
			fallback.Repaired = repaired
			fallback.FinalValidatedQuery = &fallback.Query
			return fallback, nil
		}

		remote = normalized
		applyMessageGuardrails(req.Message, &remote)
		remote.Notes = append(remote.Notes, "slm_remote_used", "loopback_validated")
		remote.SLMRaw = remoteSLMRaw
		remote.ValidationErrors = dedupeStrings(validationErrors)
		remote.Repaired = repaired
		remote.FinalValidatedQuery = &remote.Query
		pre := remote.Query
		post := remote.Query
		remote.PreSemanticQuery = &pre
		remote.PostSemanticQuery = &post
		remote.FilterSource = sourceForFilters(remote.Query.Filters, a.Name())
		return remote, nil
	}

	return fallback, nil
}

func applyMessageGuardrails(message string, res *AnalyzeResult) {
	if res == nil {
		return
	}
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return
	}
	if res.ResourceType != "order" || res.IntentCategory != contracts.IntentWISMO {
		return
	}
	if !hasNotShippedCue(lower) {
		return
	}

	filtered := make([]store.Filter, 0, len(res.Query.Filters)+1)
	for _, f := range res.Query.Filters {
		field := contracts.NormalizeField("order", f.Field)
		if field == "shipment.state" && strings.EqualFold(f.Op, "eq") {
			val := strings.TrimSpace(fmt.Sprint(f.Value))
			if strings.EqualFold(val, "Shipped") || strings.EqualFold(val, "Delivered") || strings.EqualFold(val, "Ready") {
				continue
			}
		}
		f.Field = field
		filtered = append(filtered, f)
	}
	res.Query.Filters = filtered
	applyNotShippedFilters(&res.Query)
	res.SafeEvidence = appendUnique(res.SafeEvidence, "guardrail:not_shipped")
	res.Notes = appendUnique(res.Notes, "guardrail_not_shipped_enforced")
}

type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Format string `json:"format"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
}

func (a *SLMLocalAnalyzer) tryRemoteSLM(req AnalyzeRequest) (AnalyzeResult, bool) {
	if a.endpoint == "" || a.model == "" {
		return AnalyzeResult{}, false
	}

	// Use enhanced prompt builder with schema and examples
	// Initialize if needed (for backward compatibility with tests)
	if a.promptBuilder == nil {
		a.promptBuilder = NewPromptBuilder(
			GetDefaultSchemaProvider(),
			GetDefaultExampleProvider(),
		)
	}
	prompt := a.promptBuilder.BuildRewritePrompt(req)
	rawReq, _ := json.Marshal(ollamaGenerateRequest{
		Model:  a.model,
		Prompt: prompt,
		Stream: false,
		Format: "json",
	})
	httpReq, err := http.NewRequest(http.MethodPost, a.endpoint+"/api/generate", strings.NewReader(string(rawReq)))
	if err != nil {
		return AnalyzeResult{}, false
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return AnalyzeResult{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return AnalyzeResult{}, false
	}

	var out ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return AnalyzeResult{}, false
	}
	if strings.TrimSpace(out.Response) == "" {
		return AnalyzeResult{}, false
	}

	parsed := struct {
		Intent              string         `json:"intent"`
		IntentCategory      string         `json:"intentCategory"`
		IntentSubcategory   string         `json:"intentSubcategory"`
		ResourceType        string         `json:"resourceType"`
		Confidence          float64        `json:"confidence"`
		ClarificationNeeded bool           `json:"clarificationNeeded"`
		SafeEvidence        []string       `json:"safeEvidence"`
		Query               store.QueryDSL `json:"query"`
	}{}
	if err := json.Unmarshal([]byte(out.Response), &parsed); err != nil {
		return AnalyzeResult{}, false
	}
	return AnalyzeResult{
		Query:               parsed.Query,
		Intent:              parsed.Intent,
		IntentCategory:      parsed.IntentCategory,
		IntentSubcategory:   parsed.IntentSubcategory,
		ResourceType:        parsed.ResourceType,
		Confidence:          parsed.Confidence,
		ClarificationNeeded: parsed.ClarificationNeeded,
		SafeEvidence:        parsed.SafeEvidence,
	}, true
}

func (a *SLMLocalAnalyzer) tryRepairRemoteSLM(req AnalyzeRequest, previous map[string]interface{}, validationErrors []string) (AnalyzeResult, bool) {
	if a.endpoint == "" || a.model == "" {
		return AnalyzeResult{}, false
	}
	// Use enhanced repair prompt with targeted error guidance
	// Initialize if needed (for backward compatibility with tests)
	if a.promptBuilder == nil {
		a.promptBuilder = NewPromptBuilder(
			GetDefaultSchemaProvider(),
			GetDefaultExampleProvider(),
		)
	}
	prompt := a.promptBuilder.BuildRepairPrompt(req, previous, validationErrors)
	rawReq, _ := json.Marshal(ollamaGenerateRequest{
		Model:  a.model,
		Prompt: prompt,
		Stream: false,
		Format: "json",
	})
	httpReq, err := http.NewRequest(http.MethodPost, a.endpoint+"/api/generate", strings.NewReader(string(rawReq)))
	if err != nil {
		return AnalyzeResult{}, false
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return AnalyzeResult{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return AnalyzeResult{}, false
	}
	var out ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return AnalyzeResult{}, false
	}
	parsed := struct {
		Intent              string         `json:"intent"`
		IntentCategory      string         `json:"intentCategory"`
		IntentSubcategory   string         `json:"intentSubcategory"`
		ResourceType        string         `json:"resourceType"`
		Confidence          float64        `json:"confidence"`
		ClarificationNeeded bool           `json:"clarificationNeeded"`
		SafeEvidence        []string       `json:"safeEvidence"`
		Query               store.QueryDSL `json:"query"`
	}{}
	if err := json.Unmarshal([]byte(out.Response), &parsed); err != nil {
		return AnalyzeResult{}, false
	}
	return AnalyzeResult{
		Query:               parsed.Query,
		Intent:              parsed.Intent,
		IntentCategory:      parsed.IntentCategory,
		IntentSubcategory:   parsed.IntentSubcategory,
		ResourceType:        parsed.ResourceType,
		Confidence:          parsed.Confidence,
		ClarificationNeeded: parsed.ClarificationNeeded,
		SafeEvidence:        parsed.SafeEvidence,
	}, true
}

// Legacy prompt functions - replaced by PromptBuilder
// Kept for reference, remove after confirming enhanced prompts work

// func buildRewritePrompt(message, contractVersion, resourceHint string) string {
// 	return "You are a rewrite engine. Return only JSON with fields intent,intentCategory,resourceType,confidence,clarificationNeeded,safeEvidence,query. " +
// 		"Query must include contractVersion,intentCategory,filters,sort,page and only allowed field names. " +
// 		"contractVersion=" + contractVersion + ", resourceHint=" + resourceHint + ", message=" + message
// }

// func buildRewriteRepairPrompt(message, contractVersion, resourceHint, previous string, validationErrors []string) string {
// 	return "You are a rewrite repair engine. Fix JSON output to satisfy validation rules and return only corrected JSON with fields intent,intentCategory,resourceType,confidence,clarificationNeeded,safeEvidence,query. " +
// 		"Do not add markdown. Do not explain. " +
// 		"query must include contractVersion,intentCategory,filters,sort,page with only allowed filter/sort fields and valid operators eq,neq,gt,gte,lt,lte,like,in. " +
// 		"contractVersion=" + contractVersion + ", resourceHint=" + resourceHint + ", message=" + message +
// 		", previous=" + previous + ", validationErrors=" + strings.Join(validationErrors, "|")
// }

func validateRemoteResult(in AnalyzeResult, contractVersion string) (AnalyzeResult, []string) {
	out := in
	errs := []string{}
	if out.ResourceType != "order" && out.ResourceType != "customer" {
		errs = append(errs, "invalid resourceType")
	}
	validIntent := map[string]struct{}{
		contracts.IntentDefault:        {},
		contracts.IntentWISMO:          {},
		contracts.IntentCRMProfile:     {},
		contracts.IntentReturnsRefunds: {},
	}
	if _, ok := validIntent[out.IntentCategory]; !ok {
		errs = append(errs, "invalid intentCategory")
	}

	if out.Query.ContractVersion == "" {
		out.Query.ContractVersion = contractVersion
	}
	if out.Query.IntentCategory == "" {
		out.Query.IntentCategory = out.IntentCategory
	}
	if out.IntentCategory == "" {
		out.IntentCategory = out.Query.IntentCategory
	}
	if out.Query.IntentCategory != out.IntentCategory {
		errs = append(errs, "query intentCategory mismatch")
	}

	validOps := map[string]struct{}{
		"eq": {}, "neq": {}, "gt": {}, "gte": {}, "lt": {}, "lte": {}, "like": {}, "in": {},
	}
	for _, f := range out.Query.Filters {
		if _, ok := validOps[strings.ToLower(f.Op)]; !ok {
			errs = append(errs, fmt.Sprintf("invalid operator for field %s", f.Field))
		}
		if out.ResourceType == "" || out.IntentCategory == "" || out.Query.ContractVersion == "" {
			continue
		}
		if err := contracts.ValidateField(out.ResourceType, out.Query.ContractVersion, out.IntentCategory, f.Field); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if out.Query.Sort.Field != "" && out.ResourceType != "" && out.IntentCategory != "" && out.Query.ContractVersion != "" {
		if err := contracts.ValidateField(out.ResourceType, out.Query.ContractVersion, out.IntentCategory, out.Query.Sort.Field); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if out.Query.Page.Limit <= 0 {
		out.Query.Page.Limit = 20
	}

	if (out.IntentCategory == contracts.IntentWISMO || out.IntentCategory == contracts.IntentReturnsRefunds) && out.ResourceType == "customer" {
		errs = append(errs, "resource-intent inconsistency")
	}
	return out, dedupeStrings(errs)
}

func asRawRemote(in AnalyzeResult) map[string]interface{} {
	raw := map[string]interface{}{
		"intent":              in.Intent,
		"intentCategory":      in.IntentCategory,
		"intentSubcategory":   in.IntentSubcategory,
		"resourceType":        in.ResourceType,
		"confidence":          in.Confidence,
		"clarificationNeeded": in.ClarificationNeeded,
		"safeEvidence":        in.SafeEvidence,
		"query":               in.Query,
	}
	return raw
}

func dedupeStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
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

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func normalizePrompt(message string) string {
	m := strings.ToLower(message)
	replacements := map[string]string{
		"where is my order":          "order tracking status",
		"has my order shipped":       "order shipped status",
		"tracking link doesn't work": "tracking issue",
		"last 3 orders":              "recent orders",
		"lifetime value":             "customer segment",
		"initiate refund":            "refund status",
		"eligible for a return":      "return eligible",
		"check refund status":        "refund status",
		"view recent tickets":        "support history",
		"check account notes":        "account notes",
	}
	for from, to := range replacements {
		m = strings.ReplaceAll(m, from, to)
	}
	return m
}
