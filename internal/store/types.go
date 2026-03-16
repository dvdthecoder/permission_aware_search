package store

import (
	"context"
	"encoding/json"
)

type Filter struct {
	Field string      `json:"field"`
	Op    string      `json:"op"`
	Value interface{} `json:"value"`
}

type Sort struct {
	Field string `json:"field"`
	Dir   string `json:"dir"`
}

type Page struct {
	Limit  int    `json:"limit"`
	Cursor string `json:"cursor"`
}

type QueryDSL struct {
	ContractVersion string   `json:"contractVersion"`
	IntentCategory  string   `json:"intentCategory,omitempty"`
	Filters         []Filter `json:"filters"`
	Sort            Sort     `json:"sort"`
	Page            Page     `json:"page"`
}

type Candidate struct {
	ID  string                 `json:"id"`
	Doc map[string]interface{} `json:"doc"`
}

type SearchResult struct {
	Candidates []Candidate `json:"candidates"`
	Total      int         `json:"total"`
	NextCursor string      `json:"nextCursor"`
}

type DataStore interface {
	Search(ctx context.Context, tenantID, resourceType string, query QueryDSL, maxCandidates int) (SearchResult, error)
	FetchByIDs(ctx context.Context, tenantID, resourceType string, ids []string) ([]map[string]interface{}, error)
	GetByID(ctx context.Context, tenantID, resourceType, id string) (map[string]interface{}, error)
}

func DeepCopyMap(in map[string]interface{}) map[string]interface{} {
	b, _ := json.Marshal(in)
	out := map[string]interface{}{}
	_ = json.Unmarshal(b, &out)
	return out
}
