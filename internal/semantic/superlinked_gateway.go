package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

var (
	ErrProviderRequest        = errors.New("superlinked request failed")
	ErrProviderTimeout        = errors.New("superlinked request timed out")
	ErrProviderStatus         = errors.New("superlinked returned non-success status")
	ErrProviderDecode         = errors.New("superlinked response decode failed")
	ErrInvalidProviderPayload = errors.New("superlinked invalid provider payload")
)

type GatewayRequest struct {
	TenantID        string
	Message         string
	ContractVersion string
	ResourceHint    string
	IntentCategory  string
	TopK            int
}

type GatewayResponse struct {
	CandidateIDs       []string
	Scores             []float64
	ProviderConfidence float64
	SafeEvidence       []string
	ProviderLatencyMs  int64
	ModelVersion       string
	IndexVersion       string
}

type SuperlinkedGateway interface {
	Analyze(ctx context.Context, req GatewayRequest) (GatewayResponse, error)
}

type HTTPSuperlinkedGateway struct {
	endpoint      string
	client        *http.Client
	maxCandidates int
}

type gatewayHTTPResponse struct {
	CandidateIDs       []string        `json:"candidateIds"`
	Scores             []float64       `json:"scores"`
	ProviderConfidence float64         `json:"providerConfidence"`
	SafeEvidence       []string        `json:"safeEvidence"`
	ProviderLatencyMs  int64           `json:"providerLatencyMs"`
	ModelVersion       string          `json:"modelVersion"`
	IndexVersion       string          `json:"indexVersion"`
	Query              json.RawMessage `json:"query,omitempty"`
	Confidence         float64         `json:"confidence,omitempty"`
}

type gatewayLegacyQuery struct {
	Filters []struct {
		Field string      `json:"field"`
		Op    string      `json:"op"`
		Value interface{} `json:"value"`
	} `json:"filters"`
}

func NewHTTPSuperlinkedGateway(endpoint string, timeout time.Duration, maxCandidates int) *HTTPSuperlinkedGateway {
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}
	if maxCandidates <= 0 {
		maxCandidates = 100
	}
	return &HTTPSuperlinkedGateway{
		endpoint:      strings.TrimRight(endpoint, "/"),
		client:        &http.Client{Timeout: timeout},
		maxCandidates: maxCandidates,
	}
}

func (g *HTTPSuperlinkedGateway) Analyze(ctx context.Context, req GatewayRequest) (GatewayResponse, error) {
	if g.endpoint == "" {
		return GatewayResponse{}, fmt.Errorf("%w: endpoint is empty", ErrProviderRequest)
	}
	topK := req.TopK
	if topK <= 0 {
		topK = g.maxCandidates
	}
	if topK > g.maxCandidates {
		topK = g.maxCandidates
	}

	requestBody := map[string]interface{}{
		"tenantId":        req.TenantID,
		"message":         req.Message,
		"contractVersion": req.ContractVersion,
		"resourceHint":    req.ResourceHint,
		"intentCategory":  req.IntentCategory,
		"topK":            topK,
	}
	raw, _ := json.Marshal(requestBody)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint+"/analyze", bytes.NewReader(raw))
	if err != nil {
		return GatewayResponse{}, fmt.Errorf("%w: %v", ErrProviderRequest, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := g.client.Do(httpReq)
	if err != nil {
		if isTimeoutError(err) {
			return GatewayResponse{}, fmt.Errorf("%w: %v", ErrProviderTimeout, err)
		}
		return GatewayResponse{}, fmt.Errorf("%w: %v", ErrProviderRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return GatewayResponse{}, fmt.Errorf("%w: status=%d", ErrProviderStatus, resp.StatusCode)
	}

	var decoded gatewayHTTPResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return GatewayResponse{}, fmt.Errorf("%w: %v", ErrProviderDecode, err)
	}
	if decoded.ProviderLatencyMs <= 0 {
		decoded.ProviderLatencyMs = time.Since(start).Milliseconds()
	}
	if decoded.ProviderConfidence <= 0 && decoded.Confidence > 0 {
		decoded.ProviderConfidence = decoded.Confidence
	}

	// Backward-compat bridge for local mock shape that returns `query.filters`.
	if len(decoded.CandidateIDs) == 0 && len(decoded.Query) > 0 {
		var legacy gatewayLegacyQuery
		if err := json.Unmarshal(decoded.Query, &legacy); err == nil {
			decoded.CandidateIDs = extractIDsFromFilters(legacy.Filters)
		}
	}

	normalized, err := g.normalize(decoded)
	if err != nil {
		return GatewayResponse{}, err
	}
	return normalized, nil
}

func (g *HTTPSuperlinkedGateway) normalize(in gatewayHTTPResponse) (GatewayResponse, error) {
	out := GatewayResponse{
		CandidateIDs:       make([]string, 0, len(in.CandidateIDs)),
		Scores:             make([]float64, 0, len(in.CandidateIDs)),
		ProviderConfidence: clamp01(in.ProviderConfidence),
		SafeEvidence:       dedupeStrings(in.SafeEvidence),
		ProviderLatencyMs:  in.ProviderLatencyMs,
		ModelVersion:       strings.TrimSpace(in.ModelVersion),
		IndexVersion:       strings.TrimSpace(in.IndexVersion),
	}

	candidates := make([]string, 0, len(in.CandidateIDs))
	seen := map[string]struct{}{}
	for _, id := range in.CandidateIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		candidates = append(candidates, id)
	}
	if len(candidates) == 0 {
		return GatewayResponse{}, fmt.Errorf("%w: empty candidateIds", ErrInvalidProviderPayload)
	}
	if len(candidates) > g.maxCandidates {
		candidates = candidates[:g.maxCandidates]
	}
	out.CandidateIDs = candidates

	if len(in.Scores) > 0 && len(in.Scores) != len(in.CandidateIDs) {
		return GatewayResponse{}, fmt.Errorf("%w: scores length mismatch", ErrInvalidProviderPayload)
	}

	if len(in.Scores) == 0 {
		for range out.CandidateIDs {
			out.Scores = append(out.Scores, out.ProviderConfidence)
		}
		return out, nil
	}

	for i := 0; i < len(out.CandidateIDs); i++ {
		out.Scores = append(out.Scores, clamp01(in.Scores[i]))
	}
	return out, nil
}

func extractIDsFromFilters(filters []struct {
	Field string      `json:"field"`
	Op    string      `json:"op"`
	Value interface{} `json:"value"`
}) []string {
	out := make([]string, 0)
	for _, f := range filters {
		if f.Field != "id" || strings.ToLower(f.Op) != "in" {
			continue
		}
		rawVals, ok := f.Value.([]interface{})
		if !ok {
			continue
		}
		for _, v := range rawVals {
			if str, ok := v.(string); ok && strings.TrimSpace(str) != "" {
				out = append(out, strings.TrimSpace(str))
			}
		}
	}
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}
