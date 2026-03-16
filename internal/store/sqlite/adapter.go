package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"permission_aware_search/internal/schema"
	"permission_aware_search/internal/store"
)

type Adapter struct {
	db  *sql.DB
	reg *schema.Registry
}

func NewAdapter(db *sql.DB, reg *schema.Registry) *Adapter {
	return &Adapter{db: db, reg: reg}
}

func (a *Adapter) Search(ctx context.Context, tenantID, resourceType string, query store.QueryDSL, maxCandidates int) (store.SearchResult, error) {
	table, err := a.tableForResource(resourceType)
	if err != nil {
		return store.SearchResult{}, err
	}
	if query.Page.Limit <= 0 || query.Page.Limit > maxCandidates {
		query.Page.Limit = min(20, maxCandidates)
	}
	offset := 0
	if query.Page.Cursor != "" {
		offset, _ = strconv.Atoi(query.Page.Cursor)
	}

	where, args, err := a.buildWhere(a.filtersForResource(resourceType), query.Filters)
	if err != nil {
		return store.SearchResult{}, err
	}
	args = append([]interface{}{tenantID}, args...)

	sortField := query.Sort.Field
	if sortField == "" {
		sortField = a.defaultSortField(resourceType)
	}
	sortDir := strings.ToUpper(query.Sort.Dir)
	if sortDir != "ASC" {
		sortDir = "DESC"
	}
	if _, ok := a.sortableForResource(resourceType)[sortField]; !ok {
		sortField = a.defaultSortField(resourceType)
	}
	nativeSortField, ok := a.nativeFieldForResource(resourceType, sortField)
	if !ok {
		nativeSortField, _ = a.nativeFieldForResource(resourceType, a.defaultSortField(resourceType))
	}

	countSQL := fmt.Sprintf("SELECT COUNT(1) FROM %s WHERE tenant_id = ? %s", table, where)
	var total int
	if err := a.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return store.SearchResult{}, err
	}

	limit := min(maxCandidates, max(query.Page.Limit*5, query.Page.Limit))
	querySQL := fmt.Sprintf("SELECT id, doc_json FROM %s WHERE tenant_id = ? %s ORDER BY %s %s LIMIT ? OFFSET ?", table, where, nativeSortField, sortDir)
	queryArgs := append(args, limit, offset)
	rows, err := a.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return store.SearchResult{}, err
	}
	defer rows.Close()

	out := store.SearchResult{Candidates: []store.Candidate{}, Total: total}
	for rows.Next() {
		var id string
		var docRaw string
		if err := rows.Scan(&id, &docRaw); err != nil {
			return store.SearchResult{}, err
		}
		doc := map[string]interface{}{}
		if err := json.Unmarshal([]byte(docRaw), &doc); err != nil {
			return store.SearchResult{}, err
		}
		out.Candidates = append(out.Candidates, store.Candidate{ID: id, Doc: doc})
	}

	nextOffset := offset + len(out.Candidates)
	if nextOffset < total {
		out.NextCursor = strconv.Itoa(nextOffset)
	}
	return out, rows.Err()
}

func (a *Adapter) FetchByIDs(ctx context.Context, tenantID, resourceType string, ids []string) ([]map[string]interface{}, error) {
	if len(ids) == 0 {
		return []map[string]interface{}{}, nil
	}
	table, err := a.tableForResource(resourceType)
	if err != nil {
		return nil, err
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	query := fmt.Sprintf("SELECT doc_json FROM %s WHERE tenant_id = ? AND id IN (%s)", table, placeholders)
	args := []interface{}{tenantID}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []map[string]interface{}{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		doc := map[string]interface{}{}
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, rows.Err()
}

func (a *Adapter) GetByID(ctx context.Context, tenantID, resourceType, id string) (map[string]interface{}, error) {
	table, err := a.tableForResource(resourceType)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT doc_json FROM %s WHERE tenant_id = ? AND id = ?", table)
	var raw string
	if err := a.db.QueryRowContext(ctx, query, tenantID, id).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	doc := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func (a *Adapter) tableForResource(resourceType string) (string, error) {
	return a.reg.GetTableName(resourceType)
}

func (a *Adapter) defaultSortField(resourceType string) string {
	if f, ok := a.reg.GetDefaultSortField(resourceType); ok {
		return f
	}
	// Last resort: ask the registry for the "order" default (always present).
	// Only reached on misconfiguration (unknown resourceType).
	if f, ok := a.reg.GetDefaultSortField("order"); ok {
		return f
	}
	return ""
}

func (a *Adapter) sortableForResource(resourceType string) map[string]struct{} {
	return a.reg.SortableFields(resourceType)
}

func (a *Adapter) filtersForResource(resourceType string) map[string]struct{} {
	return a.reg.FilterableFields(resourceType)
}

func (a *Adapter) buildWhere(allowed map[string]struct{}, filters []store.Filter) (string, []interface{}, error) {
	if len(filters) == 0 {
		return "", nil, nil
	}
	clauses := []string{}
	args := []interface{}{}
	for _, f := range filters {
		if _, ok := allowed[f.Field]; !ok {
			return "", nil, fmt.Errorf("unsupported filter field: %s", f.Field)
		}
		nativeField, ok := a.nativeFieldForFilterField(f.Field)
		if !ok {
			return "", nil, fmt.Errorf("unsupported filter field: %s", f.Field)
		}
		switch strings.ToLower(f.Op) {
		case "eq":
			clauses = append(clauses, fmt.Sprintf("%s = ?", nativeField))
			args = append(args, f.Value)
		case "neq":
			clauses = append(clauses, fmt.Sprintf("%s != ?", nativeField))
			args = append(args, f.Value)
		case "gt", "gte", "lt", "lte":
			op := map[string]string{"gt": ">", "gte": ">=", "lt": "<", "lte": "<="}[strings.ToLower(f.Op)]
			clauses = append(clauses, fmt.Sprintf("%s %s ?", nativeField, op))
			args = append(args, f.Value)
		case "like":
			clauses = append(clauses, fmt.Sprintf("%s LIKE ?", nativeField))
			args = append(args, f.Value)
		case "in":
			arr, ok := f.Value.([]interface{})
			if !ok || len(arr) == 0 {
				return "", nil, fmt.Errorf("invalid in operator value for field %s", f.Field)
			}
			placeholders := strings.TrimRight(strings.Repeat("?,", len(arr)), ",")
			clauses = append(clauses, fmt.Sprintf("%s IN (%s)", nativeField, placeholders))
			for _, val := range arr {
				args = append(args, val)
			}
		default:
			return "", nil, fmt.Errorf("unsupported operator: %s", f.Op)
		}
	}
	return " AND " + strings.Join(clauses, " AND "), args, nil
}

func (a *Adapter) nativeFieldForResource(resourceType, field string) (string, bool) {
	return a.reg.GetNativeColumn(resourceType, field)
}

func (a *Adapter) nativeFieldForFilterField(field string) (string, bool) {
	return a.reg.NativeColumnForField(field)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
