package identifier

import (
	"regexp"
	"strings"
)

type QueryShape string

const (
	ShapeEmpty         QueryShape = "empty"
	ShapeTypeahead     QueryShape = "typeahead_prefix"
	ShapeIdentifier    QueryShape = "identifier_token"
	ShapeContact       QueryShape = "contact_lookup"
	ShapeKeywordPhrase QueryShape = "keyword_phrase"
	ShapeSentence      QueryShape = "sentence_nl"
	ShapeUnsupported   QueryShape = "unsupported_domain"
)

type QueryShapeThresholds struct {
	ShortNoOpLen        int `json:"shortNoOpLen"`
	GenericPrefixMinLen int `json:"genericPrefixMinLen"`
	IDPrefixMinLen      int `json:"idPrefixMinLen"`
	EmailPrefixMinLen   int `json:"emailPrefixMinLen"`
}

type QueryAnalysis struct {
	OriginalInput            string               `json:"originalInput"`
	NormalizedInput          string               `json:"normalizedInput"`
	NormalizationApplied     []string             `json:"normalizationApplied,omitempty"`
	QueryShape               QueryShape           `json:"queryShape"`
	Detected                 []DetectedIdentifier `json:"detected"`
	IdentifierPatternMatched string               `json:"identifierPatternMatched,omitempty"`
	ShouldSkipSemantic       bool                 `json:"shouldSkipSemantic"`
}

var (
	reWrapper      = regexp.MustCompile(`(?i)searchQuery:\s*(.*)$`)
	reLeadingNoise = regexp.MustCompile(`^[^[:alnum:]@+]+`)
	reMultiSpace   = regexp.MustCompile(`\s+`)
	reLettersSpace = regexp.MustCompile(`^[[:alpha:]\s]+$`)
)

func AnalyzeQuery(input string, tenantID string, registry *PatternRegistry, thresholds QueryShapeThresholds) QueryAnalysis {
	normalized, applied := NormalizeInput(input)
	detected, matchedPattern := DetectWithRegistry(normalized, tenantID, registry)

	shape := classifyShape(normalized, detected, thresholds)
	if isUnsupportedDomain(normalized) && shape != ShapeIdentifier && shape != ShapeContact {
		shape = ShapeUnsupported
	}

	shouldSkipSemantic := shape == ShapeEmpty || shape == ShapeTypeahead || shape == ShapeIdentifier || shape == ShapeContact || shape == ShapeUnsupported
	if len(normalized) <= thresholds.ShortNoOpLen {
		shouldSkipSemantic = true
	}

	return QueryAnalysis{
		OriginalInput:            input,
		NormalizedInput:          normalized,
		NormalizationApplied:     applied,
		QueryShape:               shape,
		Detected:                 detected,
		IdentifierPatternMatched: matchedPattern,
		ShouldSkipSemantic:       shouldSkipSemantic,
	}
}

func NormalizeInput(input string) (string, []string) {
	applied := make([]string, 0, 4)
	normalized := strings.TrimSpace(input)
	if normalized == "" {
		return "", applied
	}

	if strings.Contains(strings.ToLower(normalized), "searchquery:") {
		if m := reWrapper.FindStringSubmatch(normalized); len(m) > 1 {
			normalized = strings.TrimSpace(m[1])
			applied = append(applied, "trim_wrapper")
		}
	}

	normalized = strings.Trim(normalized, "\"'")
	cleaned := reLeadingNoise.ReplaceAllString(normalized, "")
	if cleaned != normalized {
		normalized = cleaned
		applied = append(applied, "strip_leading_symbols")
	}

	spaceClean := reMultiSpace.ReplaceAllString(normalized, " ")
	if spaceClean != normalized {
		normalized = spaceClean
		applied = append(applied, "collapse_spaces")
	}

	normalized = strings.TrimSpace(normalized)
	if strings.ToLower(normalized) != normalized {
		applied = append(applied, "case_normalized_for_match")
	}
	return normalized, dedupeStringSlice(applied)
}

func classifyShape(normalized string, detected []DetectedIdentifier, thresholds QueryShapeThresholds) QueryShape {
	if strings.TrimSpace(normalized) == "" {
		return ShapeEmpty
	}
	if thresholds.ShortNoOpLen <= 0 {
		thresholds.ShortNoOpLen = 2
	}
	if thresholds.GenericPrefixMinLen <= 0 {
		thresholds.GenericPrefixMinLen = 3
	}
	if thresholds.IDPrefixMinLen <= 0 {
		thresholds.IDPrefixMinLen = 2
	}
	if thresholds.EmailPrefixMinLen <= 0 {
		thresholds.EmailPrefixMinLen = 3
	}

	trimmed := strings.TrimSpace(normalized)
	if len(trimmed) <= thresholds.ShortNoOpLen {
		return ShapeTypeahead
	}
	if hasTypedContact(detected) {
		return ShapeContact
	}
	if hasTypedIdentifier(detected) {
		return ShapeIdentifier
	}

	tokens := strings.Fields(strings.ToLower(trimmed))
	if len(tokens) == 0 {
		return ShapeEmpty
	}
	if len(tokens) >= 4 && (containsVerbTokens(tokens) || strings.Contains(trimmed, "?")) {
		return ShapeSentence
	}
	if len(tokens) <= 3 {
		if hasEmailPrefix(trimmed, thresholds.EmailPrefixMinLen) || isLikelyIDPrefix(trimmed, thresholds.IDPrefixMinLen) {
			return ShapeTypeahead
		}
		if containsVerbTokens(tokens) {
			return ShapeSentence
		}
		if len(tokens) >= 2 && len(trimmed) >= thresholds.GenericPrefixMinLen*2 && reLettersSpace.MatchString(trimmed) {
			return ShapeKeywordPhrase
		}
		return ShapeTypeahead
	}
	return ShapeSentence
}

func hasTypedContact(detected []DetectedIdentifier) bool {
	for _, d := range detected {
		if d.Type == TypeEmail || d.Type == TypePhone {
			return true
		}
	}
	return false
}

func isLikelyIDPrefix(in string, minLen int) bool {
	up := strings.ToUpper(strings.TrimSpace(in))
	if strings.HasPrefix(up, "ORD-") || strings.HasPrefix(up, "TRK-") || strings.HasPrefix(up, "PAY-") || strings.HasPrefix(up, "CUST-") || strings.HasPrefix(up, "PIX-") || strings.HasPrefix(up, "EFL-") {
		suffix := strings.SplitN(up, "-", 2)
		if len(suffix) < 2 {
			return false
		}
		return len(strings.TrimSpace(suffix[1])) >= minLen
	}
	return false
}

func hasEmailPrefix(in string, minLen int) bool {
	in = strings.TrimSpace(in)
	at := strings.Index(in, "@")
	if at <= 0 {
		return false
	}
	return at >= minLen
}

func isUnsupportedDomain(in string) bool {
	lower := strings.ToLower(in)
	terms := []string{
		"calendar", "poster", "flyer", "brochure", "banner", "business cards", "wedding cards",
		"print", "product", "sku", "material", "color", "style",
	}
	for _, t := range terms {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

func containsVerbTokens(tokens []string) bool {
	verbs := map[string]struct{}{
		"show": {}, "find": {}, "get": {}, "check": {}, "give": {}, "list": {},
		"what": {}, "where": {}, "has": {}, "have": {}, "is": {}, "are": {}, "why": {},
	}
	for _, t := range tokens {
		if _, ok := verbs[t]; ok {
			return true
		}
	}
	return false
}

func dedupeStringSlice(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
