package policy

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"permission_aware_search/internal/cache"
)

type SQLiteEngine struct {
	db    *sql.DB
	cache *cache.GrantsCache
}

type abacRule struct {
	Effect       string
	SubjectAttr  string
	ResourceAttr string
	Op           string
	Value        string
}

func NewSQLiteEngine(db *sql.DB) *SQLiteEngine {
	return &SQLiteEngine{
		db:    db,
		cache: cache.NewGrantsCache(30 * time.Second),
	}
}

func (e *SQLiteEngine) Evaluate(ctx context.Context, subject Subject, action, resourceType string, resource map[string]interface{}) (Decision, error) {
	if tenant, _ := resource["tenant_id"].(string); tenant != "" && tenant != subject.TenantID {
		return Decision{Allowed: false, Reason: "tenant_mismatch"}, nil
	}

	id, _ := resource["id"].(string)
	grants, err := e.grantsForSubject(ctx, subject, action, resourceType)
	if err != nil {
		return Decision{}, err
	}
	if _, ok := grants["*"]; !ok {
		if _, ok := grants[id]; !ok {
			allowByABAC, reason, err := e.evalABAC(ctx, subject, action, resourceType, resource)
			if err != nil {
				return Decision{}, err
			}
			if !allowByABAC {
				return Decision{Allowed: false, Reason: reason}, nil
			}
			return Decision{Allowed: true, Reason: "abac_allow"}, nil
		}
	}

	deny, err := e.hasDenyRule(ctx, subject, action, resourceType, resource)
	if err != nil {
		return Decision{}, err
	}
	if deny {
		return Decision{Allowed: false, Reason: "abac_deny"}, nil
	}
	return Decision{Allowed: true, Reason: "acl_grant"}, nil
}

func (e *SQLiteEngine) Explain(ctx context.Context, subject Subject, action, resourceType, resourceID string) (Decision, error) {
	res := map[string]interface{}{"id": resourceID, "tenant_id": subject.TenantID}
	return e.Evaluate(ctx, subject, action, resourceType, res)
}

func (e *SQLiteEngine) grantsForSubject(ctx context.Context, subject Subject, action, resourceType string) (map[string]struct{}, error) {
	cacheKey := fmt.Sprintf("%s:%s:%s:%s", subject.TenantID, subject.UserID, resourceType, action)
	if cached, ok := e.cache.Get(cacheKey); ok {
		return cached, nil
	}

	subjectIDs := []string{"user:" + subject.UserID}
	for _, r := range subject.Roles {
		subjectIDs = append(subjectIDs, "role:"+r)
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(subjectIDs)), ",")
	query := fmt.Sprintf(`SELECT resource_id FROM acl_grants
		WHERE tenant_id = ? AND action = ? AND resource_type = ? AND subject_id IN (%s)`, placeholders)
	args := []interface{}{subject.TenantID, action, resourceType}
	for _, sid := range subjectIDs {
		args = append(args, sid)
	}

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	e.cache.Set(cacheKey, ids)
	return ids, nil
}

func (e *SQLiteEngine) evalABAC(ctx context.Context, subject Subject, action, resourceType string, resource map[string]interface{}) (bool, string, error) {
	rules, err := e.loadRules(ctx, subject.TenantID, action, resourceType)
	if err != nil {
		return false, "", err
	}
	allowed := false
	for _, rule := range rules {
		if !matchRule(rule, subject, resource) {
			continue
		}
		if rule.Effect == "deny" {
			return false, "abac_deny", nil
		}
		if rule.Effect == "allow" {
			allowed = true
		}
	}
	if allowed {
		return true, "abac_allow", nil
	}
	return false, "no_grant", nil
}

func (e *SQLiteEngine) hasDenyRule(ctx context.Context, subject Subject, action, resourceType string, resource map[string]interface{}) (bool, error) {
	rules, err := e.loadRules(ctx, subject.TenantID, action, resourceType)
	if err != nil {
		return false, err
	}
	for _, r := range rules {
		if r.Effect == "deny" && matchRule(r, subject, resource) {
			return true, nil
		}
	}
	return false, nil
}

func (e *SQLiteEngine) loadRules(ctx context.Context, tenantID, action, resourceType string) ([]abacRule, error) {
	rows, err := e.db.QueryContext(ctx, `SELECT effect, subject_attr, resource_attr, op, value
		FROM policy_rules WHERE tenant_id = ? AND action = ? AND resource_type = ?`, tenantID, action, resourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := []abacRule{}
	for rows.Next() {
		var r abacRule
		if err := rows.Scan(&r.Effect, &r.SubjectAttr, &r.ResourceAttr, &r.Op, &r.Value); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func matchRule(rule abacRule, subject Subject, resource map[string]interface{}) bool {
	sVal := subject.Attributes[rule.SubjectAttr]
	rVal, _ := resource[rule.ResourceAttr].(string)

	switch rule.Op {
	case "equals":
		if rule.Value == "__MATCH_RESOURCE__" {
			return sVal != "" && sVal == rVal
		}
		return sVal == rule.Value
	case "not_equals":
		if rule.Value == "__MATCH_RESOURCE__" {
			return sVal != rVal
		}
		return sVal != rule.Value
	default:
		return false
	}
}
