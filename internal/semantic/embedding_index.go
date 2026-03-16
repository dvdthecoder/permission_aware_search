package semantic

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const defaultEmbeddingDims = 64

// SemanticCandidateRetriever returns top semantic candidate IDs for a message.
type SemanticCandidateRetriever interface {
	TopK(ctx context.Context, tenantID, resourceType, message string, k int) ([]string, []string, error)
}

type SQLiteEmbeddingIndex struct {
	db       *sql.DB
	dims     int
	provider EmbeddingProvider
}

func NewSQLiteEmbeddingIndex(db *sql.DB) *SQLiteEmbeddingIndex {
	return &SQLiteEmbeddingIndex{db: db, dims: defaultEmbeddingDims, provider: NewHashDemoEmbeddingProvider(defaultEmbeddingDims)}
}

func NewSQLiteEmbeddingIndexWithProvider(db *sql.DB, provider EmbeddingProvider) *SQLiteEmbeddingIndex {
	if provider == nil {
		provider = NewHashDemoEmbeddingProvider(defaultEmbeddingDims)
	}
	return &SQLiteEmbeddingIndex{db: db, dims: defaultEmbeddingDims, provider: provider}
}

func (s *SQLiteEmbeddingIndex) TopK(ctx context.Context, tenantID, resourceType, message string, k int) ([]string, []string, error) {
	if s == nil || s.db == nil || tenantID == "" || resourceType == "" || k <= 0 {
		return nil, nil, nil
	}
	start := time.Now()
	queryVec := embedText(message, s.dims)
	if s.provider != nil {
		if vecs, err := s.provider.Embed(ctx, []string{message}); err == nil && len(vecs) == 1 && len(vecs[0]) > 0 {
			queryVec = vecs[0]
		}
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT resource_id, embedding_json FROM semantic_index WHERE tenant_id = ? AND resource_type = ?`,
		tenantID,
		resourceType,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	type scored struct {
		id    string
		score float64
	}
	scoredRows := make([]scored, 0, k*4)
	usedFallbackDims := false
	for rows.Next() {
		var id string
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, nil, err
		}
		docVec := []float64{}
		if err := json.Unmarshal([]byte(raw), &docVec); err != nil {
			continue
		}
		if len(docVec) != len(queryVec) {
			usedFallbackDims = true
			continue
		}
		score := cosine(queryVec, docVec)
		if score <= 0 {
			continue
		}
		scoredRows = append(scoredRows, scored{id: id, score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	sort.Slice(scoredRows, func(i, j int) bool { return scoredRows[i].score > scoredRows[j].score })
	if len(scoredRows) > k {
		scoredRows = scoredRows[:k]
	}
	ids := make([]string, 0, len(scoredRows))
	evidence := make([]string, 0, len(scoredRows))
	if usedFallbackDims {
		evidence = append(evidence, "semantic_dim_mismatch_filtered")
	}
	evidence = append(evidence, fmt.Sprintf("semantic_vector_latency_ms=%d", time.Since(start).Milliseconds()))
	for _, item := range scoredRows {
		ids = append(ids, item.id)
		evidence = append(evidence, fmt.Sprintf("semantic_similarity:%s=%.3f", item.id, item.score))
	}
	return ids, evidence, nil
}

func EmbedForIndex(text string) string {
	vec := embedText(text, defaultEmbeddingDims)
	return marshalVec(vec)
}

func EmbedForIndexWithProvider(ctx context.Context, provider EmbeddingProvider, text string) string {
	if provider != nil {
		if vecs, err := provider.Embed(ctx, []string{text}); err == nil && len(vecs) == 1 && len(vecs[0]) > 0 {
			return marshalVec(vecs[0])
		}
	}
	vec := embedText(text, defaultEmbeddingDims)
	return marshalVec(vec)
}

func marshalVec(vec []float64) string {
	raw, _ := json.Marshal(vec)
	return string(raw)
}

func embedText(text string, dims int) []float64 {
	out := make([]float64, dims)
	lower := strings.ToLower(text)
	tokens := strings.Fields(lower)
	if len(tokens) == 0 {
		return out
	}
	for _, tok := range tokens {
		if tok == "" {
			continue
		}
		h1 := hashString(tok)
		h2 := hashString(reverse(tok))
		i1 := h1 % dims
		i2 := h2 % dims
		out[i1] += 1.0
		out[i2] += 0.5
	}
	norm := 0.0
	for _, v := range out {
		norm += v * v
	}
	if norm == 0 {
		return out
	}
	norm = math.Sqrt(norm)
	for i := range out {
		out[i] /= norm
	}
	return out
}

func cosine(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	sum := 0.0
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func hashString(s string) int {
	h := 2166136261
	for i := 0; i < len(s); i++ {
		h ^= int(s[i])
		h *= 16777619
	}
	if h < 0 {
		return -h
	}
	return h
}

func reverse(s string) string {
	buf := []byte(s)
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
