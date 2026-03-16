package semantic

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"permission_aware_search/internal/identifier"
)

type DetailedSemanticCandidateRetriever interface {
	SemanticCandidateRetriever
	Retrieve(ctx context.Context, req RetrievalRequest) (RetrievalResult, error)
}

type RetrievalRequest struct {
	TenantID       string
	ResourceType   string
	Message        string
	QueryShape     string
	IntentCategory string
	TopK           int
}

type RetrievalResult struct {
	CandidateIDs   []string
	Evidence       []string
	Provider       string
	ModelVersion   string
	FallbackChain  []string
	GateReason     string
	Strategy       string
	LatencyMs      int64
	ScoredEvidence []string
}

type HybridCandidateRetriever struct {
	db         *sql.DB
	vector     SemanticCandidateRetriever
	config     RetrievalConfig
	model      string
	providerID string
}

func NewHybridCandidateRetriever(db *sql.DB, vector SemanticCandidateRetriever, configPath string, provider EmbeddingProvider) *HybridCandidateRetriever {
	providerID := "unknown"
	model := "unknown"
	if provider != nil {
		providerID = provider.Name()
		model = provider.Model()
	}
	return &HybridCandidateRetriever{
		db:         db,
		vector:     vector,
		config:     loadRetrievalConfig(configPath),
		model:      model,
		providerID: providerID,
	}
}

func (h *HybridCandidateRetriever) TopK(ctx context.Context, tenantID, resourceType, message string, k int) ([]string, []string, error) {
	res, err := h.Retrieve(ctx, RetrievalRequest{
		TenantID:     tenantID,
		ResourceType: resourceType,
		Message:      message,
		TopK:         k,
	})
	if err != nil {
		return nil, nil, err
	}
	return res.CandidateIDs, res.Evidence, nil
}

func (h *HybridCandidateRetriever) Retrieve(ctx context.Context, req RetrievalRequest) (RetrievalResult, error) {
	start := time.Now()
	if req.TopK <= 0 {
		req.TopK = h.config.FusionCap
	}

	shape := strings.TrimSpace(strings.ToLower(req.QueryShape))
	if shape == "" {
		analysis := identifier.AnalyzeQuery(req.Message, req.TenantID, nil, identifier.QueryShapeThresholds{})
		shape = string(analysis.QueryShape)
	}
	weights := h.config.Weights
	if shape == "identifier_token" || shape == "contact_lookup" || shape == "typeahead_prefix" {
		weights = identifierHeavyWeights()
	}

	scores := map[string]float64{}
	evidence := map[string][]string{}

	idsID, evID := h.identifierCandidates(ctx, req)
	for rank, id := range idsID {
		addScore(scores, evidence, id, weights.Identifier*rankScore(rank), "identifier", evID)
	}

	idsLex, evLex := h.lexicalCandidates(ctx, req)
	for rank, id := range idsLex {
		addScore(scores, evidence, id, weights.Lexical*rankScore(rank), "lexical", evLex)
	}

	idsVec, evVec := h.vectorCandidates(ctx, req)
	for rank, id := range idsVec {
		addScore(scores, evidence, id, weights.Vector*rankScore(rank), "vector", evVec)
	}

	type scored struct {
		id    string
		score float64
	}
	list := make([]scored, 0, len(scores))
	for id, s := range scores {
		list = append(list, scored{id: id, score: s})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })

	limit := req.TopK
	if h.config.FusionCap > 0 && limit > h.config.FusionCap {
		limit = h.config.FusionCap
	}
	if len(list) > limit {
		list = list[:limit]
	}

	outIDs := make([]string, 0, len(list))
	outEvidence := make([]string, 0, len(list)+4)
	outScores := make([]string, 0, len(list))
	for _, item := range list {
		outIDs = append(outIDs, item.id)
		outScores = append(outScores, fmt.Sprintf("%s=%.4f", item.id, item.score))
		if parts, ok := evidence[item.id]; ok && len(parts) > 0 {
			outEvidence = append(outEvidence, fmt.Sprintf("fusion:%s:%s", item.id, strings.Join(parts, "|")))
		}
	}
	outEvidence = append(outEvidence, fmt.Sprintf("weights:id=%.2f,lex=%.2f,vec=%.2f", weights.Identifier, weights.Lexical, weights.Vector))

	strategy := "hybrid_fusion"
	if len(idsVec) == 0 && len(idsLex) > 0 {
		strategy = "lexical_only_fallback"
	}
	if len(idsVec) == 0 && len(idsLex) == 0 && len(idsID) > 0 {
		strategy = "identifier_only"
	}
	if len(outIDs) == 0 && len(idsVec) > 0 {
		strategy = "vector_only_fallback"
	}

	return RetrievalResult{
		CandidateIDs:   outIDs,
		Evidence:       outEvidence,
		Provider:       h.providerID,
		ModelVersion:   h.model,
		FallbackChain:  []string{"hybrid_fusion", "lexical_only", "identifier_only", "deterministic_query_only"},
		GateReason:     "",
		Strategy:       strategy,
		LatencyMs:      time.Since(start).Milliseconds(),
		ScoredEvidence: outScores,
	}, nil
}

func addScore(scores map[string]float64, evidence map[string][]string, id string, score float64, source string, sourceEvidence []string) {
	id = strings.TrimSpace(id)
	if id == "" || score <= 0 {
		return
	}
	scores[id] += score
	ev := source
	if len(sourceEvidence) > 0 {
		ev += ":" + strings.Join(sourceEvidence, ",")
	}
	evidence[id] = append(evidence[id], ev)
}

func rankScore(rank int) float64 {
	return 1.0 / float64(rank+1)
}

func (h *HybridCandidateRetriever) identifierCandidates(ctx context.Context, req RetrievalRequest) ([]string, []string) {
	if h.db == nil {
		return nil, nil
	}
	plan := identifier.BuildResolutionPlanWithConfig(req.Message, req.TenantID, req.ResourceType, nil, identifier.QueryShapeThresholds{})
	ids := make([]string, 0, 32)
	for _, g := range plan.Groups {
		if g.ResourceType != req.ResourceType {
			continue
		}
		sqlField, ok := toSQLField(g.MatchField, req.ResourceType)
		if !ok {
			continue
		}
		q := fmt.Sprintf("SELECT id FROM %s WHERE tenant_id=? AND %s %s ? LIMIT 30", tableFor(req.ResourceType), sqlField, opToSQL(g.Operator))
		rows, err := h.db.QueryContext(ctx, q, req.TenantID, g.Value)
		if err != nil {
			continue
		}
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		_ = rows.Close()
	}
	return dedupeIDs(ids), []string{"identifier_lookup"}
}

func (h *HybridCandidateRetriever) lexicalCandidates(ctx context.Context, req RetrievalRequest) ([]string, []string) {
	if h.db == nil {
		return nil, nil
	}
	tokens := lexicalTokens(req.Message)
	if len(tokens) == 0 {
		return nil, nil
	}
	fields := lexicalFieldsFor(req.ResourceType)
	if len(fields) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, h.config.TopKLexical)
	for _, tok := range tokens {
		for _, f := range fields {
			q := fmt.Sprintf("SELECT id FROM %s WHERE tenant_id=? AND lower(%s) LIKE ? LIMIT 20", tableFor(req.ResourceType), f)
			rows, err := h.db.QueryContext(ctx, q, req.TenantID, "%"+strings.ToLower(tok)+"%")
			if err != nil {
				continue
			}
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					ids = append(ids, id)
				}
			}
			_ = rows.Close()
		}
	}
	if len(ids) > h.config.TopKLexical {
		ids = ids[:h.config.TopKLexical]
	}
	return dedupeIDs(ids), []string{"lexical_lookup"}
}

func (h *HybridCandidateRetriever) vectorCandidates(ctx context.Context, req RetrievalRequest) ([]string, []string) {
	if h.vector == nil {
		return nil, nil
	}
	k := h.config.TopKVector
	if req.TopK > 0 && req.TopK < k {
		k = req.TopK
	}
	ids, evidence, err := h.vector.TopK(ctx, req.TenantID, req.ResourceType, req.Message, k)
	if err != nil {
		return nil, []string{"vector_error"}
	}
	return ids, evidence
}

func lexicalTokens(message string) []string {
	parts := strings.Fields(strings.ToLower(message))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(p, " ,.;:!?()[]{}\"'")
		if len(p) < 3 {
			continue
		}
		if _, err := strconv.Atoi(p); err == nil && len(p) < 5 {
			continue
		}
		out = append(out, p)
	}
	return out
}

func lexicalFieldsFor(resourceType string) []string {
	if resourceType == "customer" {
		return []string{"customer_number", "email", "first_name", "last_name", "vip_tier", "customer_group"}
	}
	return []string{"order_number", "tracking_id", "payment_reference", "customer_email", "order_state", "shipment_state", "payment_state"}
}

func toSQLField(field, resourceType string) (string, bool) {
	f := strings.ToLower(strings.TrimSpace(field))
	switch f {
	case "order.number":
		return "order_number", true
	case "shipment.tracking_id":
		return "tracking_id", true
	case "payment.reference":
		return "payment_reference", true
	case "order.customer_email":
		return "customer_email", true
	case "order.customer_id":
		return "customer_id", true
	case "customer.number":
		return "customer_number", true
	case "customer.email":
		return "email", true
	case "id", "order.id", "customer.id":
		return "id", true
	default:
		_ = resourceType
		return "", false
	}
}

func tableFor(resourceType string) string {
	if resourceType == "customer" {
		return "customers_docs"
	}
	return "orders_docs"
}

func opToSQL(op string) string {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "like":
		return "LIKE"
	default:
		return "="
	}
}

func dedupeIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
