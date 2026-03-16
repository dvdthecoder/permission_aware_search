package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"permission_aware_search/internal/policy"
)

// SubjectFromRequest is a lightweight SuperTokens-compatible adapter for demo use.
// In production, replace this with actual session validation via SuperTokens OSS SDK.
func SubjectFromRequest(r *http.Request) policy.Subject {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		userID = r.Header.Get("X-Supertokens-User-Id")
	}
	if userID == "" {
		userID = "alice"
	}

	tenantID := r.Header.Get("X-Tenant-Id")
	if tenantID == "" {
		tenantID = "tenant-a"
	}

	rolesHeader := r.Header.Get("X-Roles")
	if rolesHeader == "" {
		rolesHeader = "sales_rep"
	}
	parts := strings.Split(rolesHeader, ",")
	roles := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			roles = append(roles, trimmed)
		}
	}

	attrs := map[string]string{}
	if raw := r.Header.Get("X-User-Attrs"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &attrs)
	}
	if _, ok := attrs["region"]; !ok {
		attrs["region"] = r.Header.Get("X-User-Region")
	}
	if attrs["region"] == "" {
		attrs["region"] = "west"
	}

	return policy.Subject{
		UserID:     userID,
		TenantID:   tenantID,
		Roles:      roles,
		Attributes: attrs,
	}
}
