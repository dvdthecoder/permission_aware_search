package search

import (
	"context"
	"testing"

	"permission_aware_search/internal/policy"
	"permission_aware_search/internal/store"
)

type fakeStore struct {
	res store.SearchResult
}

func (f fakeStore) Search(ctx context.Context, tenantID, resourceType string, query store.QueryDSL, maxCandidates int) (store.SearchResult, error) {
	return f.res, nil
}
func (f fakeStore) FetchByIDs(ctx context.Context, tenantID, resourceType string, ids []string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (f fakeStore) GetByID(ctx context.Context, tenantID, resourceType, id string) (map[string]interface{}, error) {
	return nil, nil
}

type fakePolicy struct{}

func (f fakePolicy) Evaluate(ctx context.Context, subject policy.Subject, action, resourceType string, resource map[string]interface{}) (policy.Decision, error) {
	id, _ := resource["id"].(string)
	if id == "ord-1" {
		return policy.Decision{Allowed: true, Reason: "test_allow"}, nil
	}
	return policy.Decision{Allowed: false, Reason: "test_deny"}, nil
}
func (f fakePolicy) Explain(ctx context.Context, subject policy.Subject, action, resourceType, resourceID string) (policy.Decision, error) {
	return policy.Decision{Allowed: false, Reason: "n/a"}, nil
}

func TestSearchRedactionBehavior(t *testing.T) {
	svc := NewService(fakeStore{res: store.SearchResult{
		Total: 2,
		Candidates: []store.Candidate{
			{ID: "ord-1", Doc: map[string]interface{}{"id": "ord-1", "tenant_id": "tenant-a", "status": "open"}},
			{ID: "ord-2", Doc: map[string]interface{}{"id": "ord-2", "tenant_id": "tenant-a", "status": "open"}},
		},
	}}, fakePolicy{})

	resp, err := svc.Search(context.Background(), policy.Subject{UserID: "alice", TenantID: "tenant-a"}, "order", store.QueryDSL{
		ContractVersion: "v1",
		Filters:         []store.Filter{{Field: "status", Op: "eq", Value: "open"}},
		Page:            store.Page{Limit: 20},
	})
	if err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	if resp.AuthorizedCount != 1 {
		t.Fatalf("expected 1 authorized item, got %d", resp.AuthorizedCount)
	}
	if resp.HiddenCount != 1 {
		t.Fatalf("expected 1 hidden item, got %d", resp.HiddenCount)
	}
	if len(resp.RedactedPlaceholders) == 0 {
		t.Fatalf("expected redacted placeholder")
	}
	if resp.RedactedPlaceholders[0].ResourceID != "ord-2" {
		t.Fatalf("expected hidden placeholder to expose only id ord-2, got %s", resp.RedactedPlaceholders[0].ResourceID)
	}
	if resp.ResultReasonCode != "VISIBLE_RESULTS" {
		t.Fatalf("expected VISIBLE_RESULTS reason code, got %s", resp.ResultReasonCode)
	}
}

func TestSearchNoMatchReasonCode(t *testing.T) {
	svc := NewService(fakeStore{res: store.SearchResult{
		Total:      0,
		Candidates: []store.Candidate{},
	}}, fakePolicy{})

	resp, err := svc.Search(context.Background(), policy.Subject{UserID: "alice", TenantID: "tenant-a"}, "order", store.QueryDSL{
		ContractVersion: "v1",
		Filters:         []store.Filter{{Field: "status", Op: "eq", Value: "open"}},
		Page:            store.Page{Limit: 20},
	})
	if err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	if resp.ResultReasonCode != "NO_MATCH_IN_TENANT" {
		t.Fatalf("expected NO_MATCH_IN_TENANT, got %s", resp.ResultReasonCode)
	}
}
