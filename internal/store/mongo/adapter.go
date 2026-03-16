package mongo

import (
	"context"
	"errors"

	"permission_aware_search/internal/store"
)

// Adapter is a production parity stub. Implement with MongoDB driver in production.
type Adapter struct{}

func NewAdapter() *Adapter { return &Adapter{} }

func (a *Adapter) Search(ctx context.Context, tenantID, resourceType string, query store.QueryDSL, maxCandidates int) (store.SearchResult, error) {
	return store.SearchResult{}, errors.New("mongo adapter not implemented in demo")
}

func (a *Adapter) FetchByIDs(ctx context.Context, tenantID, resourceType string, ids []string) ([]map[string]interface{}, error) {
	return nil, errors.New("mongo adapter not implemented in demo")
}

func (a *Adapter) GetByID(ctx context.Context, tenantID, resourceType, id string) (map[string]interface{}, error) {
	return nil, errors.New("mongo adapter not implemented in demo")
}
