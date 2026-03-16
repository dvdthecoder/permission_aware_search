package search

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"permission_aware_search/internal/contracts"
	"permission_aware_search/internal/policy"
	"permission_aware_search/internal/store"
)

type RedactedPlaceholder struct {
	ResourceID         string `json:"resourceId"`
	Type               string `json:"type"`
	ReasonCode         string `json:"reasonCode"`
	RequestAccessToken string `json:"requestAccessToken"`
}

type SearchResponse struct {
	Items                 []map[string]interface{} `json:"items"`
	AuthorizedCount       int                      `json:"authorizedCount"`
	HiddenCount           int                      `json:"hiddenCount"`
	ResultReasonCode      string                   `json:"resultReasonCode"`
	VisibilityNotice      string                   `json:"visibilityNotice"`
	SuggestedNextActions  []string                 `json:"suggestedNextActions,omitempty"`
	RedactedPlaceholders  []RedactedPlaceholder    `json:"redactedPlaceholders"`
	VisibilityMode        string                   `json:"visibilityMode"`
	ContractVersion       string                   `json:"contractVersion"`
	NextCursor            string                   `json:"nextCursor,omitempty"`
	LatencyMs             int64                    `json:"latencyMs"`
	ScopeCappedCandidates bool                     `json:"scopeCappedCandidates"`
}

type Service struct {
	store               store.DataStore
	policy              policy.Engine
	maxCandidateWindow  int
	requestTokenSecret  []byte
	maxPlaceholderCount int
}

func NewService(ds store.DataStore, pe policy.Engine) *Service {
	return &Service{
		store:               ds,
		policy:              pe,
		maxCandidateWindow:  500,
		requestTokenSecret:  []byte("change-me-demo-secret"),
		maxPlaceholderCount: 3,
	}
}

func (s *Service) Search(ctx context.Context, subject policy.Subject, resourceType string, query store.QueryDSL) (SearchResponse, error) {
	start := time.Now()
	if query.ContractVersion == "" {
		query.ContractVersion = contracts.ContractVersionV2
	}
	if query.IntentCategory == "" {
		query.IntentCategory = contracts.IntentDefault
	}
	if query.ContractVersion == contracts.ContractVersionV2 {
		for i := range query.Filters {
			query.Filters[i].Field = contracts.NormalizeField(resourceType, query.Filters[i].Field)
		}
		if query.Sort.Field != "" {
			query.Sort.Field = contracts.NormalizeField(resourceType, query.Sort.Field)
		}
	}
	for _, f := range query.Filters {
		if err := contracts.ValidateField(resourceType, query.ContractVersion, query.IntentCategory, f.Field); err != nil {
			return SearchResponse{}, err
		}
	}
	if query.Sort.Field != "" {
		if err := contracts.ValidateField(resourceType, query.ContractVersion, query.IntentCategory, query.Sort.Field); err != nil {
			return SearchResponse{}, err
		}
	}

	res, err := s.store.Search(ctx, subject.TenantID, resourceType, query, s.maxCandidateWindow)
	if err != nil {
		return SearchResponse{}, err
	}

	allowed := make([]map[string]interface{}, 0, len(res.Candidates))
	hiddenIDs := make([]string, 0, len(res.Candidates))
	for _, candidate := range res.Candidates {
		decision, err := s.policy.Evaluate(ctx, subject, "view", resourceType, candidate.Doc)
		if err != nil {
			return SearchResponse{}, err
		}
		if decision.Allowed {
			allowed = append(allowed, store.DeepCopyMap(candidate.Doc))
			continue
		}
		hiddenIDs = append(hiddenIDs, candidate.ID)
	}
	allowedInWindow := len(allowed)

	limit := query.Page.Limit
	if limit <= 0 {
		limit = 20
	}
	if len(allowed) > limit {
		allowed = allowed[:limit]
	}

	hiddenInWindow := len(res.Candidates) - allowedInWindow
	if hiddenInWindow < 0 {
		hiddenInWindow = 0
	}

	redacted := make([]RedactedPlaceholder, 0, min(hiddenInWindow, s.maxPlaceholderCount))
	for i := 0; i < min(hiddenInWindow, s.maxPlaceholderCount); i++ {
		redacted = append(redacted, RedactedPlaceholder{
			ResourceID:         hiddenIDs[i],
			Type:               resourceType,
			ReasonCode:         "INSUFFICIENT_PERMISSION",
			RequestAccessToken: s.requestToken(resourceType, query, subject),
		})
	}

	reasonCode, notice, suggestions := deriveResultReason(res.Total, len(res.Candidates), len(allowed), hiddenInWindow, res.Total > len(res.Candidates))

	return SearchResponse{
		Items:                 allowed,
		AuthorizedCount:       len(allowed),
		HiddenCount:           hiddenInWindow,
		ResultReasonCode:      reasonCode,
		VisibilityNotice:      notice,
		SuggestedNextActions:  suggestions,
		RedactedPlaceholders:  redacted,
		VisibilityMode:        "REDACTED_PARTIAL",
		ContractVersion:       query.ContractVersion,
		NextCursor:            res.NextCursor,
		LatencyMs:             time.Since(start).Milliseconds(),
		ScopeCappedCandidates: res.Total > len(res.Candidates),
	}, nil
}

func deriveResultReason(totalMatched, candidatesInWindow, authorizedInWindow, hiddenInWindow int, scopeCapped bool) (string, string, []string) {
	if authorizedInWindow > 0 {
		return "VISIBLE_RESULTS", "Matching records found and visible for your current access scope.", nil
	}
	if hiddenInWindow > 0 {
		return "MATCHES_EXIST_BUT_NOT_VISIBLE", "Matching records exist but are not visible with your current permissions.", []string{
			"Use a persona/role with broader access.",
			"Verify tenant and region scope.",
			"Request additional access for this query context.",
		}
	}
	if totalMatched == 0 {
		return "NO_MATCH_IN_TENANT", "No matching records found in the current tenant for this query.", []string{
			"Check identifier/value and query filters.",
			"Confirm the tenant context is correct.",
		}
	}
	if candidatesInWindow == 0 || scopeCapped {
		return "NO_VISIBLE_RESULTS_FOR_CURRENT_SCOPE", "No visible records in the current search scope for your access context.", []string{
			"Try a narrower or different filter.",
			"Try a persona/role with broader access.",
		}
	}
	return "NO_VISIBLE_RESULTS_FOR_CURRENT_SCOPE", "No visible records for the current access scope.", []string{
		"Try a persona/role with broader access.",
	}
}

func (s *Service) Detail(ctx context.Context, subject policy.Subject, resourceType, id string) (map[string]interface{}, bool, error) {
	resource, err := s.store.GetByID(ctx, subject.TenantID, resourceType, id)
	if err != nil {
		return nil, false, err
	}
	if resource == nil {
		return nil, false, nil
	}
	decision, err := s.policy.Evaluate(ctx, subject, "view", resourceType, resource)
	if err != nil {
		return nil, false, err
	}
	if !decision.Allowed {
		return nil, false, nil
	}
	return resource, true, nil
}

func (s *Service) requestToken(resourceType string, query store.QueryDSL, subject policy.Subject) string {
	payload := map[string]interface{}{
		"resourceType": resourceType,
		"contract":     query.ContractVersion,
		"tenantId":     subject.TenantID,
		"userId":       subject.UserID,
		"exp":          time.Now().Add(10 * time.Minute).Unix(),
	}
	raw, _ := json.Marshal(payload)
	mac := hmac.New(sha256.New, s.requestTokenSecret)
	_, _ = mac.Write(raw)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (s *Service) Explain(ctx context.Context, subject policy.Subject, action, resourceType, resourceID string) (policy.Decision, error) {
	return s.policy.Explain(ctx, subject, action, resourceType, resourceID)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Service) ValidateRequestAccessToken(token string, subject policy.Subject) error {
	parts := make([]string, 0, 2)
	for _, p := range []rune(token) {
		_ = p
	}
	parts = splitToken(token)
	if len(parts) != 2 {
		return fmt.Errorf("invalid token")
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("invalid token")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("invalid token")
	}
	mac := hmac.New(sha256.New, s.requestTokenSecret)
	_, _ = mac.Write(raw)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return fmt.Errorf("invalid token")
	}
	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("invalid token")
	}
	if payload["tenantId"] != subject.TenantID || payload["userId"] != subject.UserID {
		return fmt.Errorf("invalid token")
	}
	return nil
}

func splitToken(token string) []string {
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			return []string{token[:i], token[i+1:]}
		}
	}
	return nil
}
