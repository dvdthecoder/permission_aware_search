package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"permission_aware_search/internal/auth"
	"permission_aware_search/internal/contracts"
	"permission_aware_search/internal/identifier"
	"permission_aware_search/internal/observability"
	"permission_aware_search/internal/policy"
	"permission_aware_search/internal/search"
	"permission_aware_search/internal/semantic"
	"permission_aware_search/internal/store"
)

type Server struct {
	search            *search.Service
	metrics           *observability.Metrics
	db                *sql.DB
	semantic          *semantic.Router
	idFastPathEnabled bool
	idGroupedEnabled  bool
	idPatterns        *identifier.PatternRegistry
	shapeThresholds   identifier.QueryShapeThresholds
}

type queryInterpretPayload struct {
	Message         string `json:"message"`
	Provider        string `json:"provider,omitempty"`
	ResourceHint    string `json:"resourceHint,omitempty"`
	ContractVersion string `json:"contractVersion,omitempty"`
	Debug           bool   `json:"debug,omitempty"`
}

var traceCounter uint64

func NewServer(searchSvc *search.Service, metrics *observability.Metrics, db *sql.DB, semanticRouter *semantic.Router) *Server {
	return &Server{
		search:            searchSvc,
		metrics:           metrics,
		db:                db,
		semantic:          semanticRouter,
		idFastPathEnabled: boolEnvOrDefault("IDENTIFIER_FAST_PATH_ENABLED", true),
		idGroupedEnabled:  boolEnvOrDefault("IDENTIFIER_GROUPED_RESPONSE_ENABLED", true),
		idPatterns:        identifier.LoadPatternRegistry(envOrDefault("IDENTIFIER_PATTERNS_PATH", "config/identifier_patterns.json")),
		shapeThresholds:   identifier.LoadThresholds(envOrDefault("QUERY_SHAPE_THRESHOLDS_PATH", "config/query_shape_thresholds.json")),
	}
}

func (s *Server) Routes() stdhttp.Handler {
	mux := stdhttp.NewServeMux()
	mux.HandleFunc("GET /api/me", s.handleMe)
	mux.HandleFunc("POST /api/search/orders", s.handleSearch("order"))
	mux.HandleFunc("POST /api/search/customers", s.handleSearch("customer"))
	mux.HandleFunc("POST /api/query/interpret", s.handleQueryInterpret)
	mux.HandleFunc("POST /api/chat/query", s.handleQueryInterpret)
	mux.HandleFunc("GET /api/orders/{id}", s.handleDetail("order"))
	mux.HandleFunc("GET /api/customers/{id}", s.handleDetail("customer"))
	mux.HandleFunc("GET /api/permissions/explain", s.handleExplain)
	mux.HandleFunc("GET /api/metrics", s.handleMetrics)
	mux.HandleFunc("GET /api/admin/seed-stats", s.handleSeedStats)
	mux.HandleFunc("POST /api/mock/superlinked/analyze", s.handleSuperlinkedMockAnalyze)

	return withCORS(withJSONError(mux))
}

func (s *Server) handleMe(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	subject := auth.SubjectFromRequest(r)
	writeJSON(w, stdhttp.StatusOK, subject)
}

func (s *Server) handleSearch(resourceType string) stdhttp.HandlerFunc {
	return func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		start := time.Now()
		traceID := traceIDFromRequest(r)
		w.Header().Set("X-Trace-Id", traceID)
		subject := auth.SubjectFromRequest(r)

		var req store.QueryDSL
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, stdhttp.StatusBadRequest, "invalid json")
			return
		}

		resp, err := s.search.Search(r.Context(), subject, resourceType, req)
		if err != nil {
			if strings.Contains(err.Error(), "FIELD_NOT_ALLOWED") {
				writeError(w, stdhttp.StatusBadRequest, err.Error())
				return
			}
			writeError(w, stdhttp.StatusInternalServerError, err.Error())
			return
		}
		s.metrics.RecordSearch(time.Since(start), resp.HiddenCount)
		writeJSON(w, stdhttp.StatusOK, map[string]interface{}{
			"items":                 resp.Items,
			"authorizedCount":       resp.AuthorizedCount,
			"hiddenCount":           resp.HiddenCount,
			"resultReasonCode":      resp.ResultReasonCode,
			"visibilityNotice":      resp.VisibilityNotice,
			"suggestedNextActions":  resp.SuggestedNextActions,
			"redactedPlaceholders":  resp.RedactedPlaceholders,
			"visibilityMode":        resp.VisibilityMode,
			"contractVersion":       resp.ContractVersion,
			"nextCursor":            resp.NextCursor,
			"latencyMs":             resp.LatencyMs,
			"scopeCappedCandidates": resp.ScopeCappedCandidates,
			"traceId":               traceID,
			"debug": map[string]interface{}{
				"flow": []map[string]string{
					{"stage": "ingress", "status": "ok"},
					{"stage": "auth_subject_built", "status": "ok"},
					{"stage": "contract_validation", "status": "ok"},
					{"stage": "datastore_search", "status": "ok"},
					{"stage": "policy_filter", "status": "ok"},
					{"stage": "response_composed", "status": "ok"},
				},
			},
		})
	}
}

func (s *Server) handleQueryInterpret(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	start := time.Now()
	traceID := traceIDFromRequest(r)
	w.Header().Set("X-Trace-Id", traceID)
	subject := auth.SubjectFromRequest(r)
	payload := queryInterpretPayload{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, stdhttp.StatusBadRequest, "invalid json")
		return
	}
	if payload.ContractVersion == "" {
		payload.ContractVersion = "v2"
	}
	analysis := identifier.AnalyzeQuery(payload.Message, subject.TenantID, s.idPatterns, s.shapeThresholds)
	plan := identifier.BuildResolutionPlanWithConfig(payload.Message, subject.TenantID, payload.ResourceHint, s.idPatterns, s.shapeThresholds)

	if strings.TrimSpace(analysis.NormalizedInput) == "" || len([]rune(strings.TrimSpace(analysis.NormalizedInput))) <= s.shapeThresholds.ShortNoOpLen {
		writeJSON(w, stdhttp.StatusOK, map[string]interface{}{
			"intent":                   "short_query_noop",
			"intentCategory":           contracts.IntentDefault,
			"queryShape":               analysis.QueryShape,
			"normalizationApplied":     analysis.NormalizationApplied,
			"identifierPatternMatched": analysis.IdentifierPatternMatched,
			"pathTaken":                "no_op_short_query",
			"resolutionMode":           "identifier_fast_path",
			"generatedQuery":           nil,
			"groupedMatches":           []interface{}{},
			"items":                    []interface{}{},
			"authorizedCount":          0,
			"hiddenCount":              0,
			"resultReasonCode":         "NO_OP_SHORT_QUERY",
			"visibilityNotice":         "Query too short. Please type at least 3 characters.",
			"suggestedNextActions": []string{
				"Type at least 3 characters.",
				"Paste an identifier (order/tracking/payment/customer/email).",
			},
			"redactedPlaceholders":       []interface{}{},
			"semanticProvider":           "identifier-resolver",
			"semanticNotes":              []string{"no_op_short_query"},
			"safeEvidence":               []string{"query_shape:" + string(analysis.QueryShape)},
			"rewriteIntentProvider":      "",
			"rewriteIntentModelVersion":  "",
			"rewriteIntentFallbackChain": []string{},
			"rewriteIntentGateReason":    "",
			"retrievalProvider":          "",
			"retrievalStrategy":          "",
			"retrievalFallbackChain":     []string{},
			"retrievalGateReason":        "",
			"retrievalLatencyMs":         0,
			"retrievalEvidence":          []string{},
			"latencyMs":                  time.Since(start).Milliseconds(),
			"traceId":                    traceID,
		})
		return
	}

	if analysis.QueryShape == identifier.ShapeUnsupported {
		writeJSON(w, stdhttp.StatusOK, map[string]interface{}{
			"intent":                   "unsupported_domain",
			"intentCategory":           contracts.IntentDefault,
			"queryShape":               analysis.QueryShape,
			"normalizationApplied":     analysis.NormalizationApplied,
			"identifierPatternMatched": analysis.IdentifierPatternMatched,
			"pathTaken":                "intent_semantic_path",
			"resolutionMode":           "intent_semantic_path",
			"generatedQuery":           nil,
			"items":                    []interface{}{},
			"authorizedCount":          0,
			"hiddenCount":              0,
			"resultReasonCode":         "UNSUPPORTED_DOMAIN",
			"visibilityNotice":         "Query appears product/catalog-oriented and is outside order/customer support scope.",
			"suggestedNextActions": []string{
				"Search by order/customer identifiers or contact details.",
				"Use order/customer operational wording (status, shipment, payment, returns).",
			},
			"redactedPlaceholders":       []interface{}{},
			"semanticProvider":           "intent-router",
			"semanticNotes":              []string{"unsupported_domain"},
			"safeEvidence":               []string{"query_shape:unsupported_domain"},
			"rewriteIntentProvider":      "",
			"rewriteIntentModelVersion":  "",
			"rewriteIntentFallbackChain": []string{},
			"rewriteIntentGateReason":    "",
			"retrievalProvider":          "",
			"retrievalStrategy":          "",
			"retrievalFallbackChain":     []string{},
			"retrievalGateReason":        "",
			"retrievalLatencyMs":         0,
			"retrievalEvidence":          []string{},
			"latencyMs":                  time.Since(start).Milliseconds(),
			"traceId":                    traceID,
		})
		return
	}

	if s.idFastPathEnabled && s.idGroupedEnabled {
		if fastResp, ok, err := s.tryIdentifierFastPath(r.Context(), subject, payload, traceID, start, plan); err != nil {
			writeError(w, stdhttp.StatusInternalServerError, err.Error())
			return
		} else if ok {
			writeJSON(w, stdhttp.StatusOK, fastResp)
			return
		}
	}

	analyzed, err := s.semantic.Analyze(r.Context(), semantic.AnalyzeRequest{
		Message:          payload.Message,
		ProviderOverride: payload.Provider,
		ResourceHint:     payload.ResourceHint,
		ContractVersion:  payload.ContractVersion,
		TenantID:         subject.TenantID,
		QueryShape:       string(analysis.QueryShape),
	})
	if err != nil {
		writeError(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}

	if analyzed.ClarificationNeeded {
		debugFlow := buildSemanticDebugFlow(analyzed.Notes, true)
		debugObj := map[string]interface{}{
			"filterSource": analyzed.FilterSource,
		}
		if payload.Debug {
			debugObj["flow"] = debugFlow
			debugObj["traceId"] = traceID
			debugObj["retrieval"] = map[string]interface{}{
				"provider":      analyzed.RetrievalProvider,
				"strategy":      analyzed.RetrievalStrategy,
				"fallbackChain": analyzed.RetrievalFallbackChain,
				"gateReason":    analyzed.RetrievalGateReason,
				"latencyMs":     analyzed.RetrievalLatencyMs,
				"evidence":      analyzed.RetrievalEvidence,
				"scores":        analyzed.RetrievalScores,
			}
			debugObj["rewrite"] = map[string]interface{}{
				"originalMessage":            analyzed.OriginalMessage,
				"rewrittenMessage":           analyzed.RewrittenMessage,
				"normalizedInput":            analyzed.NormalizedInput,
				"normalizationApplied":       analyzed.NormalizationApplied,
				"extractedSlots":             analyzed.ExtractedSlots,
				"preSemanticQuery":           analyzed.PreSemanticQuery,
				"postSemanticQuery":          analyzed.PostSemanticQuery,
				"slmRaw":                     analyzed.SLMRaw,
				"validationErrors":           analyzed.ValidationErrors,
				"repaired":                   analyzed.Repaired,
				"finalValidatedQuery":        analyzed.FinalValidatedQuery,
				"generatedQuery":             analyzed.Query,
				"intent":                     analyzed.Intent,
				"intentCategory":             analyzed.IntentCategory,
				"intentSubcategory":          analyzed.IntentSubcategory,
				"resourceType":               analyzed.ResourceType,
				"rewriteIntentProvider":      analyzed.RewriteIntentProvider,
				"rewriteIntentModelVersion":  analyzed.RewriteIntentModelVersion,
				"rewriteIntentFallbackChain": analyzed.RewriteIntentFallbackChain,
				"rewriteIntentGateReason":    analyzed.RewriteIntentGateReason,
			}
		}
		writeJSON(w, stdhttp.StatusOK, map[string]interface{}{
			"intent":                   analyzed.Intent,
			"intentCategory":           analyzed.IntentCategory,
			"intentSubcategory":        analyzed.IntentSubcategory,
			"queryShape":               analysis.QueryShape,
			"normalizationApplied":     analysis.NormalizationApplied,
			"identifierPatternMatched": analysis.IdentifierPatternMatched,
			"pathTaken":                "intent_semantic_path",
			"resolutionMode":           "intent_semantic_path",
			"generatedQuery":           analyzed.Query,
			"clarificationRequired":    true,
			"resultReasonCode":         "CLARIFICATION_REQUIRED",
			"visibilityNotice":         "Request is ambiguous. Please specify order/customer and at least one filter.",
			"suggestedNextActions": []string{
				"Add explicit entity (order/customer) and one filter.",
				"Use an identifier such as order number, tracking ID, or email.",
			},
			"authorizedCount":            0,
			"hiddenCount":                0,
			"semanticProvider":           analyzed.Provider,
			"semanticNotes":              analyzed.Notes,
			"safeEvidence":               analyzed.SafeEvidence,
			"rewriteIntentProvider":      analyzed.RewriteIntentProvider,
			"rewriteIntentModelVersion":  analyzed.RewriteIntentModelVersion,
			"rewriteIntentFallbackChain": analyzed.RewriteIntentFallbackChain,
			"rewriteIntentGateReason":    analyzed.RewriteIntentGateReason,
			"retrievalProvider":          analyzed.RetrievalProvider,
			"retrievalStrategy":          analyzed.RetrievalStrategy,
			"retrievalFallbackChain":     analyzed.RetrievalFallbackChain,
			"retrievalGateReason":        analyzed.RetrievalGateReason,
			"retrievalLatencyMs":         analyzed.RetrievalLatencyMs,
			"retrievalEvidence":          analyzed.RetrievalEvidence,
			"latencyMs":                  time.Since(start).Milliseconds(),
			"traceId":                    traceID,
			"debug":                      debugObj,
		})
		return
	}

	resourceType := analyzed.ResourceType
	resp, err := s.search.Search(r.Context(), subject, resourceType, analyzed.Query)
	if err != nil {
		writeError(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}

	debugFlow := buildSemanticDebugFlow(analyzed.Notes, false)
	debugObj := map[string]interface{}{
		"filterSource": analyzed.FilterSource,
	}
	if payload.Debug {
		debugObj["flow"] = debugFlow
		debugObj["traceId"] = traceID
		debugObj["retrieval"] = map[string]interface{}{
			"provider":      analyzed.RetrievalProvider,
			"strategy":      analyzed.RetrievalStrategy,
			"fallbackChain": analyzed.RetrievalFallbackChain,
			"gateReason":    analyzed.RetrievalGateReason,
			"latencyMs":     analyzed.RetrievalLatencyMs,
			"evidence":      analyzed.RetrievalEvidence,
			"scores":        analyzed.RetrievalScores,
		}
		debugObj["rewrite"] = map[string]interface{}{
			"originalMessage":            analyzed.OriginalMessage,
			"rewrittenMessage":           analyzed.RewrittenMessage,
			"normalizedInput":            analyzed.NormalizedInput,
			"normalizationApplied":       analyzed.NormalizationApplied,
			"extractedSlots":             analyzed.ExtractedSlots,
			"preSemanticQuery":           analyzed.PreSemanticQuery,
			"postSemanticQuery":          analyzed.PostSemanticQuery,
			"slmRaw":                     analyzed.SLMRaw,
			"validationErrors":           analyzed.ValidationErrors,
			"repaired":                   analyzed.Repaired,
			"finalValidatedQuery":        analyzed.FinalValidatedQuery,
			"generatedQuery":             analyzed.Query,
			"intent":                     analyzed.Intent,
			"intentCategory":             analyzed.IntentCategory,
			"intentSubcategory":          analyzed.IntentSubcategory,
			"resourceType":               analyzed.ResourceType,
			"rewriteIntentProvider":      analyzed.RewriteIntentProvider,
			"rewriteIntentModelVersion":  analyzed.RewriteIntentModelVersion,
			"rewriteIntentFallbackChain": analyzed.RewriteIntentFallbackChain,
			"rewriteIntentGateReason":    analyzed.RewriteIntentGateReason,
		}
	}

	writeJSON(w, stdhttp.StatusOK, map[string]interface{}{
		"intent":                     analyzed.Intent,
		"intentCategory":             analyzed.IntentCategory,
		"intentSubcategory":          analyzed.IntentSubcategory,
		"queryShape":                 analysis.QueryShape,
		"normalizationApplied":       analysis.NormalizationApplied,
		"identifierPatternMatched":   analysis.IdentifierPatternMatched,
		"pathTaken":                  "intent_semantic_path",
		"resolutionMode":             "intent_semantic_path",
		"generatedQuery":             analyzed.Query,
		"items":                      resp.Items,
		"authorizedCount":            resp.AuthorizedCount,
		"hiddenCount":                resp.HiddenCount,
		"resultReasonCode":           resp.ResultReasonCode,
		"visibilityNotice":           resp.VisibilityNotice,
		"suggestedNextActions":       resp.SuggestedNextActions,
		"redactedPlaceholders":       resp.RedactedPlaceholders,
		"answer":                     fmt.Sprintf("Found %d visible %s records.", resp.AuthorizedCount, resourceType),
		"semanticProvider":           analyzed.Provider,
		"semanticNotes":              analyzed.Notes,
		"safeEvidence":               analyzed.SafeEvidence,
		"rewriteIntentProvider":      analyzed.RewriteIntentProvider,
		"rewriteIntentModelVersion":  analyzed.RewriteIntentModelVersion,
		"rewriteIntentFallbackChain": analyzed.RewriteIntentFallbackChain,
		"rewriteIntentGateReason":    analyzed.RewriteIntentGateReason,
		"retrievalProvider":          analyzed.RetrievalProvider,
		"retrievalStrategy":          analyzed.RetrievalStrategy,
		"retrievalFallbackChain":     analyzed.RetrievalFallbackChain,
		"retrievalGateReason":        analyzed.RetrievalGateReason,
		"retrievalLatencyMs":         analyzed.RetrievalLatencyMs,
		"retrievalEvidence":          analyzed.RetrievalEvidence,
		"latencyMs":                  time.Since(start).Milliseconds(),
		"traceId":                    traceID,
		"debug":                      debugObj,
	})
}

type groupedMatch struct {
	ResourceType         string                       `json:"resourceType"`
	MatchField           string                       `json:"matchField"`
	AuthorizedCount      int                          `json:"authorizedCount"`
	HiddenCount          int                          `json:"hiddenCount"`
	Items                []map[string]interface{}     `json:"items"`
	RedactedPlaceholders []search.RedactedPlaceholder `json:"redactedPlaceholders"`
}

func (s *Server) tryIdentifierFastPath(
	ctx context.Context,
	subject policy.Subject,
	payload queryInterpretPayload,
	traceID string,
	start time.Time,
	plan identifier.ResolutionPlan,
) (map[string]interface{}, bool, error) {
	if !plan.ShouldUseFastPath {
		return nil, false, nil
	}

	type result struct {
		spec groupedMatch
		err  error
	}
	ch := make(chan result, len(plan.Groups))
	var wg sync.WaitGroup
	for _, g := range plan.Groups {
		group := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			q := store.QueryDSL{
				ContractVersion: payload.ContractVersion,
				IntentCategory:  contracts.IntentDefault,
				Filters:         []store.Filter{{Field: group.MatchField, Op: group.Operator, Value: group.Value}},
				Sort:            store.Sort{Field: "created_at", Dir: "desc"},
				Page:            store.Page{Limit: 20},
			}
			resp, err := s.search.Search(ctx, subject, group.ResourceType, q)
			if err != nil {
				ch <- result{err: err}
				return
			}
			ch <- result{spec: groupedMatch{
				ResourceType:         group.ResourceType,
				MatchField:           group.MatchField,
				AuthorizedCount:      resp.AuthorizedCount,
				HiddenCount:          resp.HiddenCount,
				Items:                resp.Items,
				RedactedPlaceholders: resp.RedactedPlaceholders,
			}}
		}()
	}
	wg.Wait()
	close(ch)

	groups := make([]groupedMatch, 0, len(plan.Groups))
	totalAuthorized := 0
	totalHidden := 0
	for r := range ch {
		if r.err != nil {
			return nil, false, r.err
		}
		groups = append(groups, r.spec)
		totalAuthorized += r.spec.AuthorizedCount
		totalHidden += r.spec.HiddenCount
	}

	flow := []map[string]string{
		{"stage": "ingress", "status": "ok"},
		{"stage": "auth_subject_built", "status": "ok"},
		{"stage": "identifier_detection_completed", "status": "ok"},
		{"stage": "identifier_parallel_lookup_completed", "status": "ok"},
		{"stage": "grouped_resolution_composed", "status": "ok"},
	}

	debugObj := map[string]interface{}{
		"resolutionMode":           "identifier_fast_path",
		"identifierDetection":      plan.Detected,
		"queryShape":               plan.QueryShape,
		"normalizationApplied":     plan.NormalizationApplied,
		"identifierPatternMatched": plan.IdentifierPatternMatched,
		"groupingSummary": map[string]interface{}{
			"groupCount":      len(groups),
			"authorizedCount": totalAuthorized,
			"hiddenCount":     totalHidden,
		},
	}
	if payload.Debug {
		debugObj["flow"] = flow
		debugObj["traceId"] = traceID
	}

	notice := "Identifier lookup resolved grouped matches."
	if totalAuthorized == 0 && totalHidden == 0 {
		notice = "No matching records found for the supplied identifier."
	} else if totalAuthorized == 0 && totalHidden > 0 {
		notice = "Matches found but hidden by permissions."
	}

	return map[string]interface{}{
		"intent":                     "identifier_lookup",
		"intentCategory":             contracts.IntentDefault,
		"queryShape":                 plan.QueryShape,
		"normalizationApplied":       plan.NormalizationApplied,
		"identifierPatternMatched":   plan.IdentifierPatternMatched,
		"pathTaken":                  pathTakenForShape(plan.QueryShape),
		"generatedQuery":             nil,
		"resolutionMode":             "identifier_fast_path",
		"detectedIdentifiers":        plan.Detected,
		"groupedMatches":             groups,
		"items":                      []interface{}{},
		"authorizedCount":            totalAuthorized,
		"hiddenCount":                totalHidden,
		"resultReasonCode":           resultReasonForFastPath(totalAuthorized, totalHidden),
		"visibilityNotice":           notice,
		"suggestedNextActions":       suggestedNextActionsForFastPath(totalAuthorized, totalHidden),
		"redactedPlaceholders":       []interface{}{},
		"answer":                     fmt.Sprintf("Found %d visible records across %d grouped matches.", totalAuthorized, len(groups)),
		"semanticProvider":           "identifier-resolver",
		"semanticNotes":              []string{"identifier_fast_path"},
		"safeEvidence":               []string{"identifier_lookup", "query_shape:" + string(plan.QueryShape)},
		"rewriteIntentProvider":      "",
		"rewriteIntentModelVersion":  "",
		"rewriteIntentFallbackChain": []string{},
		"rewriteIntentGateReason":    "",
		"retrievalProvider":          "",
		"retrievalStrategy":          "identifier_only",
		"retrievalFallbackChain":     []string{},
		"retrievalGateReason":        "",
		"retrievalLatencyMs":         0,
		"retrievalEvidence":          []string{},
		"latencyMs":                  time.Since(start).Milliseconds(),
		"traceId":                    traceID,
		"debug":                      debugObj,
	}, true, nil
}

func resultReasonForFastPath(authorized, hidden int) string {
	if authorized > 0 {
		return "VISIBLE_RESULTS"
	}
	if hidden > 0 {
		return "MATCHES_EXIST_BUT_NOT_VISIBLE"
	}
	return "NO_MATCH_IN_TENANT"
}

func suggestedNextActionsForFastPath(authorized, hidden int) []string {
	if authorized > 0 {
		return nil
	}
	if hidden > 0 {
		return []string{
			"Use a persona/role with broader access.",
			"Verify tenant and region scope.",
			"Request additional access for this query context.",
		}
	}
	return []string{
		"Check identifier/value and query filters.",
		"Confirm the tenant context is correct.",
	}
}

func pathTakenForShape(shape identifier.QueryShape) string {
	switch shape {
	case identifier.ShapeIdentifier:
		return "identifier_fast_path"
	case identifier.ShapeContact:
		return "contact_fast_path"
	case identifier.ShapeTypeahead:
		return "typeahead_fast_path"
	default:
		return "identifier_fast_path"
	}
}

func (s *Server) handleSuperlinkedMockAnalyze(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	payload := struct {
		Message         string `json:"message"`
		ContractVersion string `json:"contractVersion"`
		ResourceHint    string `json:"resourceHint,omitempty"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, stdhttp.StatusBadRequest, "invalid json")
		return
	}
	if payload.ContractVersion == "" {
		payload.ContractVersion = "v2"
	}
	analyzed, err := s.semantic.Analyze(r.Context(), semantic.AnalyzeRequest{
		Message:          payload.Message,
		ContractVersion:  payload.ContractVersion,
		ResourceHint:     payload.ResourceHint,
		TenantID:         auth.SubjectFromRequest(r).TenantID,
		ProviderOverride: "superlinked-mock",
	})
	if err != nil {
		writeError(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stdhttp.StatusOK, map[string]interface{}{
		"intent":              analyzed.Intent,
		"intentCategory":      analyzed.IntentCategory,
		"intentSubcategory":   analyzed.IntentSubcategory,
		"resourceType":        analyzed.ResourceType,
		"confidence":          analyzed.Confidence,
		"clarificationNeeded": analyzed.ClarificationNeeded,
		"safeEvidence":        analyzed.SafeEvidence,
		"retrievalProvider":   analyzed.RetrievalProvider,
		"retrievalStrategy":   analyzed.RetrievalStrategy,
		"retrievalEvidence":   analyzed.RetrievalEvidence,
		"query":               analyzed.Query,
	})
}

func (s *Server) handleDetail(resourceType string) stdhttp.HandlerFunc {
	return func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		subject := auth.SubjectFromRequest(r)
		id := r.PathValue("id")
		doc, ok, err := s.search.Detail(r.Context(), subject, resourceType, id)
		if err != nil {
			writeError(w, stdhttp.StatusInternalServerError, err.Error())
			return
		}
		if !ok || doc == nil {
			writeError(w, stdhttp.StatusForbidden, "forbidden")
			return
		}
		writeJSON(w, stdhttp.StatusOK, doc)
	}
}

func (s *Server) handleExplain(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	subject := auth.SubjectFromRequest(r)
	action := r.URL.Query().Get("action")
	if action == "" {
		action = "view"
	}
	resourceType := r.URL.Query().Get("resourceType")
	resourceID := r.URL.Query().Get("resourceId")
	if resourceType == "" || resourceID == "" {
		writeError(w, stdhttp.StatusBadRequest, "resourceType and resourceId are required")
		return
	}
	decision, err := s.search.Explain(r.Context(), subject, action, resourceType, resourceID)
	if err != nil {
		writeError(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stdhttp.StatusOK, decision)
}

func (s *Server) handleMetrics(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
	writeJSON(w, stdhttp.StatusOK, s.metrics.Snapshot())
}

func (s *Server) handleSeedStats(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	tenant := r.URL.Query().Get("tenantId")
	if tenant == "" {
		tenant = "tenant-a"
	}

	orders, err := s.countByTenant("orders_docs", tenant)
	if err != nil {
		writeError(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	customers, err := s.countByTenant("customers_docs", tenant)
	if err != nil {
		writeError(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}

	var grants int
	if err := s.db.QueryRow(
		`SELECT COUNT(1) FROM acl_grants WHERE tenant_id = ?`,
		tenant,
	).Scan(&grants); err != nil {
		writeError(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}

	var rules int
	if err := s.db.QueryRow(
		`SELECT COUNT(1) FROM policy_rules WHERE tenant_id = ?`,
		tenant,
	).Scan(&rules); err != nil {
		writeError(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, stdhttp.StatusOK, map[string]interface{}{
		"tenantId":  tenant,
		"orders":    orders,
		"customers": customers,
		"aclGrants": grants,
		"abacRules": rules,
	})
}

func (s *Server) countByTenant(table, tenant string) (int, error) {
	query := fmt.Sprintf(`SELECT COUNT(1) FROM %s WHERE tenant_id = ?`, table)
	var count int
	if err := s.db.QueryRow(query, tenant).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func withJSONError(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		next.ServeHTTP(w, r)
	})
}

func withCORS(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-User-Id,X-Tenant-Id,X-Roles,X-User-Attrs,X-User-Region")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == stdhttp.MethodOptions {
			w.WriteHeader(stdhttp.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w stdhttp.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w stdhttp.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{"error": message})
}

func traceIDFromRequest(r *stdhttp.Request) string {
	if incoming := strings.TrimSpace(r.Header.Get("X-Trace-Id")); incoming != "" {
		return incoming
	}
	seq := atomic.AddUint64(&traceCounter, 1)
	return "trc-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.FormatUint(seq, 36)
}

func buildSemanticDebugFlow(notes []string, clarification bool) []map[string]string {
	flow := []map[string]string{
		{"stage": "ingress", "status": "ok"},
		{"stage": "auth_subject_built", "status": "ok"},
		{"stage": "rewrite_completed", "status": "ok"},
	}
	if hasSemanticNote(notes, "superlinked_gateway_ok") {
		flow = append(flow, map[string]string{"stage": "superlinked_gateway_called", "status": "ok"})
	}
	if reason, ok := superlinkedFallbackReason(notes); ok {
		flow = append(flow, map[string]string{"stage": "superlinked_serving_gate", "status": "warning"})
		flow = append(flow, map[string]string{"stage": "semantic_refinement_completed", "status": "warning"})
		flow = append(flow, map[string]string{"stage": "semantic_fallback_reason:" + reason, "status": "warning"})
	} else if hasSemanticNote(notes, "superlinked_served") {
		flow = append(flow, map[string]string{"stage": "superlinked_serving_gate", "status": "ok"})
		flow = append(flow, map[string]string{"stage": "semantic_refinement_completed", "status": "ok"})
	} else {
		flow = append(flow, map[string]string{"stage": "semantic_refinement_completed", "status": "ok"})
	}
	if clarification {
		flow = append(flow, map[string]string{"stage": "clarification_required", "status": "ok"})
		return flow
	}
	flow = append(flow, map[string]string{"stage": "datastore_search", "status": "ok"})
	flow = append(flow, map[string]string{"stage": "policy_filter", "status": "ok"})
	flow = append(flow, map[string]string{"stage": "response_composed", "status": "ok"})
	return flow
}

func hasSemanticNote(notes []string, expected string) bool {
	for _, n := range notes {
		if n == expected {
			return true
		}
	}
	return false
}

func superlinkedFallbackReason(notes []string) (string, bool) {
	const prefix = "superlinked_gated_fallback:"
	for _, n := range notes {
		if strings.HasPrefix(n, prefix) {
			return strings.TrimPrefix(n, prefix), true
		}
	}
	return "", false
}

func boolEnvOrDefault(name string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envOrDefault(name, fallback string) string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	return raw
}
