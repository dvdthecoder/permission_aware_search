package identifier

import (
	"regexp"
	"strings"
)

var (
	reOrderNumber    = regexp.MustCompile(`(?i)\bORD-\d{6}\b|\bord-\d{5}\b`)
	reTrackingID     = regexp.MustCompile(`(?i)\bTRK-\d{8}\b|\bTB-TRK-\d{6}\b`)
	rePaymentRef     = regexp.MustCompile(`(?i)\bPAY-\d{8}\b`)
	reCustomerNumber = regexp.MustCompile(`(?i)\bCUST-\d{6}\b|\bcust-\d{5}\b|\bTB-CUST-\d{5}\b`)
	reEmail          = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
	rePhone          = regexp.MustCompile(`\+?\d[\d\s\-()]{7,}\d`)
	reAlphaNumOnly   = regexp.MustCompile(`^[a-zA-Z0-9@\-\._:+#]+$`)
)

func Detect(input string) []DetectedIdentifier {
	out, _ := DetectWithRegistry(input, "", nil)
	return out
}

func DetectWithRegistry(input, tenantID string, registry *PatternRegistry) ([]DetectedIdentifier, string) {
	lower := strings.ToLower(strings.TrimSpace(input))
	if lower == "" {
		return nil, ""
	}
	out := make([]DetectedIdentifier, 0, 4)
	patternMatched := ""
	add := func(idType IdentifierType, v string, c float64) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		out = append(out, DetectedIdentifier{
			Type:            idType,
			NormalizedValue: normalizeByType(idType, v),
			Confidence:      c,
		})
	}

	patterns := defaultCompiledPatterns()
	if registry != nil {
		patterns = registry.patternsForTenant(tenantID)
	}
	for _, p := range patterns {
		matches := p.Matcher.FindAllString(input, -1)
		for _, m := range matches {
			conf := 0.95
			if p.IDType == TypeEmail {
				conf = 0.9
				m = strings.ToLower(m)
			}
			if p.IDType == TypePhone {
				conf = 0.85
			}
			add(p.IDType, m, conf)
			if patternMatched == "" {
				patternMatched = p.Name
			}
		}
	}

	if len(out) == 0 {
		token := strings.TrimSpace(input)
		if token != "" && reAlphaNumOnly.MatchString(token) && len(strings.Fields(token)) <= 3 {
			out = append(out, DetectedIdentifier{
				Type:            TypeUnknownToken,
				NormalizedValue: token,
				Confidence:      0.4,
			})
		}
	}
	return dedupeDetected(out), patternMatched
}

func ShouldUseFastPath(input string, detected []DetectedIdentifier) bool {
	if len(detected) == 0 {
		return false
	}
	parts := strings.Fields(strings.TrimSpace(input))
	if len(parts) <= 6 && hasTypedIdentifier(detected) {
		return true
	}
	if startsWithKnownPrefix(strings.TrimSpace(input)) {
		return true
	}
	if len(parts) <= 6 && !containsVerb(strings.ToLower(input)) {
		return true
	}
	return false
}

func hasTypedIdentifier(detected []DetectedIdentifier) bool {
	for _, d := range detected {
		if d.Type != TypeUnknownToken {
			return true
		}
	}
	return false
}

func startsWithKnownPrefix(input string) bool {
	up := strings.ToUpper(input)
	return strings.HasPrefix(up, "ORD-") || strings.HasPrefix(up, "TRK-") || strings.HasPrefix(up, "PAY-") || strings.HasPrefix(up, "CUST-")
}

func containsVerb(lower string) bool {
	verbs := []string{"show", "find", "get", "check", "give", "list", "what", "where", "has", "have", "is", "are"}
	for _, v := range verbs {
		if strings.Contains(lower, " "+v+" ") || strings.HasPrefix(lower, v+" ") {
			return true
		}
	}
	return false
}

func normalizeByType(idType IdentifierType, raw string) string {
	switch idType {
	case TypeOrderNumber:
		up := strings.ToUpper(raw)
		if strings.HasPrefix(up, "ORD-") && len(up) == len("ORD-")+5 {
			return "ORD-0" + up[len("ORD-"):]
		}
		return up
	case TypeTrackingID, TypePaymentRef, TypeCustomerNumber:
		return strings.ToUpper(raw)
	case TypeEmail:
		return strings.ToLower(raw)
	default:
		return raw
	}
}

func dedupeDetected(in []DetectedIdentifier) []DetectedIdentifier {
	seen := map[string]struct{}{}
	out := make([]DetectedIdentifier, 0, len(in))
	for _, d := range in {
		key := string(d.Type) + "|" + d.NormalizedValue
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, d)
	}
	return out
}
