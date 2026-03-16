package identifier

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

type PatternConfig struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Regex string `json:"regex"`
}

type PatternFile struct {
	Tenants map[string][]PatternConfig `json:"tenants"`
}

type compiledPattern struct {
	Name    string
	IDType  IdentifierType
	Matcher *regexp.Regexp
}

type PatternRegistry struct {
	TenantPatterns map[string][]compiledPattern
}

func LoadPatternRegistry(path string) *PatternRegistry {
	reg := &PatternRegistry{
		TenantPatterns: map[string][]compiledPattern{
			"default": defaultCompiledPatterns(),
		},
	}

	if strings.TrimSpace(path) == "" {
		return reg
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return reg
	}
	cfg := PatternFile{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return reg
	}

	for tenant, defs := range cfg.Tenants {
		compiled := make([]compiledPattern, 0, len(defs))
		for _, d := range defs {
			idType, ok := patternTypeToIdentifierType(d.Type)
			if !ok {
				continue
			}
			re, err := regexp.Compile(d.Regex)
			if err != nil {
				continue
			}
			compiled = append(compiled, compiledPattern{
				Name:    d.Name,
				IDType:  idType,
				Matcher: re,
			})
		}
		if len(compiled) > 0 {
			reg.TenantPatterns[tenant] = compiled
		}
	}
	return reg
}

func (r *PatternRegistry) patternsForTenant(tenantID string) []compiledPattern {
	if r == nil {
		return defaultCompiledPatterns()
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID != "" {
		if p, ok := r.TenantPatterns[tenantID]; ok && len(p) > 0 {
			return p
		}
	}
	if p, ok := r.TenantPatterns["default"]; ok && len(p) > 0 {
		return p
	}
	return defaultCompiledPatterns()
}

func patternTypeToIdentifierType(patternType string) (IdentifierType, bool) {
	switch strings.ToLower(strings.TrimSpace(patternType)) {
	case "order_number":
		return TypeOrderNumber, true
	case "tracking_id":
		return TypeTrackingID, true
	case "payment_reference":
		return TypePaymentRef, true
	case "customer_number":
		return TypeCustomerNumber, true
	case "email":
		return TypeEmail, true
	case "phone":
		return TypePhone, true
	case "unknown_token":
		return TypeUnknownToken, true
	default:
		return "", false
	}
}

func defaultCompiledPatterns() []compiledPattern {
	return []compiledPattern{
		{Name: "ord_default", IDType: TypeOrderNumber, Matcher: reOrderNumber},
		{Name: "trk_default", IDType: TypeTrackingID, Matcher: reTrackingID},
		{Name: "pay_default", IDType: TypePaymentRef, Matcher: rePaymentRef},
		{Name: "cust_default", IDType: TypeCustomerNumber, Matcher: reCustomerNumber},
		{Name: "email_default", IDType: TypeEmail, Matcher: reEmail},
		{Name: "phone_default", IDType: TypePhone, Matcher: rePhone},
		{Name: "pix_pattern", IDType: TypeOrderNumber, Matcher: regexp.MustCompile(`(?i)\bPIX-[A-Z]{3}-\d+\b`)},
		{Name: "efl_pattern", IDType: TypeOrderNumber, Matcher: regexp.MustCompile(`(?i)\bEFL-[A-Z]{3}-\d+\b`)},
		{Name: "prefixed_numeric", IDType: TypeOrderNumber, Matcher: regexp.MustCompile(`(?i)\b[a-z]\d{6,}\b`)},
		{Name: "numeric_long", IDType: TypeOrderNumber, Matcher: regexp.MustCompile(`\b\d{6,}\b`)},
	}
}
