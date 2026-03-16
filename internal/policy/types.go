package policy

import "context"

type Subject struct {
	UserID     string            `json:"userId"`
	TenantID   string            `json:"tenantId"`
	Roles      []string          `json:"roles"`
	Attributes map[string]string `json:"attributes"`
}

type Decision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

type Engine interface {
	Evaluate(ctx context.Context, subject Subject, action, resourceType string, resource map[string]interface{}) (Decision, error)
	Explain(ctx context.Context, subject Subject, action, resourceType, resourceID string) (Decision, error)
}
