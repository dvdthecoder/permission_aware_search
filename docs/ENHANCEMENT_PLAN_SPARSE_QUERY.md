# Sparse Query Handling & Query Expansion Enhancement Plan

## Problem Statement
Support agents often type **incomplete, abbreviated, or ambiguous queries** due to time pressure. Current system struggles with:
- "ord delayed" → Should find orders with shipment.state != Shipped/Delivered
- "cust vip issue" → Should find VIP customers with recent support tickets
- "pkg not rcvd" → Should understand package = order, rcvd = received/delivered

## Examples of Sparse Queries from Support Agents

### Category 1: Abbreviations
```
- "ord" → order
- "cust" → customer
- "pkg" → package → order
- "trk" → tracking
- "shp" → shipment/shipped
- "pmt" → payment
- "ref" → refund
- "rma" → return
```

### Category 2: Incomplete Phrases
```
- "delayed" → shipment.state = Delayed OR created_at < (now - 7 days) AND shipment.state != Delivered
- "failed" → payment.state = Failed
- "not shipped" → shipment.state != Shipped
- "vip" → customer.vip_tier IN (gold, platinum, diamond)
```

### Category 3: Domain-Specific Jargon
```
- "stuck in processing" → order.state = Open AND created_at < (now - 3 days)
- "payment bounce" → payment.state = Failed
- "held at customs" → shipment.state = Delayed (if carrier tracking indicates customs)
- "reship needed" → Requires order duplication intent
```

## Proposed Solution: Multi-Layer Query Expansion

### 1. Lexical Expansion (Synonym Mapping)

```go
// /internal/semantic/query_expander.go

type QueryExpander interface {
    Expand(input string) ExpandedQuery
}

type ExpandedQuery struct {
    Original      string
    Normalized    string
    Expansions    []string
    Synonyms      map[string][]string
    InferredTerms []string
    Confidence    float64
}

type LexicalExpander struct {
    synonymMap    map[string][]string
    abbreviations map[string]string
    domainTerms   map[string]DomainTerm
}

type DomainTerm struct {
    Term        string
    FieldMaps   []FieldMapping
    Confidence  float64
}

type FieldMapping struct {
    Field    string
    Operator string
    Value    interface{}
}

func NewLexicalExpander() *LexicalExpander {
    return &LexicalExpander{
        // Abbreviation mappings
        abbreviations: map[string]string{
            "ord":  "order",
            "ordr": "order",
            "cust": "customer",
            "pkg":  "package",
            "trk":  "tracking",
            "shp":  "shipment",
            "pmt":  "payment",
            "ref":  "refund",
            "rma":  "return",
            "rcvd": "received",
            "dlvd": "delivered",
            "proc": "processing",
        },

        // Synonym expansions
        synonymMap: map[string][]string{
            "order": {"purchase", "transaction", "sale"},
            "package": {"order", "shipment", "parcel"},
            "tracking": {"track", "status", "whereabouts"},
            "delayed": {"late", "behind", "slow"},
            "failed": {"declined", "rejected", "unsuccessful"},
            "customer": {"client", "buyer", "shopper"},
            "vip": {"premium", "gold", "platinum", "priority"},
        },

        // Domain-specific term mappings
        domainTerms: map[string]DomainTerm{
            "delayed": {
                Term: "delayed",
                FieldMaps: []FieldMapping{
                    {Field: "shipment.state", Operator: "eq", Value: "Delayed"},
                    {Field: "shipment.state", Operator: "neq", Value: "Delivered"},
                    {Field: "order.created_at", Operator: "lt", Value: "now-7d"},
                },
                Confidence: 0.85,
            },
            "stuck": {
                Term: "stuck",
                FieldMaps: []FieldMapping{
                    {Field: "order.state", Operator: "eq", Value: "Open"},
                    {Field: "order.created_at", Operator: "lt", Value: "now-3d"},
                },
                Confidence: 0.8,
            },
            "not shipped": {
                Term: "not shipped",
                FieldMaps: []FieldMapping{
                    {Field: "shipment.state", Operator: "neq", Value: "Shipped"},
                    {Field: "shipment.state", Operator: "neq", Value: "Delivered"},
                },
                Confidence: 0.95,
            },
            "vip": {
                Term: "vip",
                FieldMaps: []FieldMapping{
                    {Field: "customer.vip_tier", Operator: "in", Value: []string{"gold", "platinum", "diamond"}},
                },
                Confidence: 0.9,
            },
            "payment bounce": {
                Term: "payment bounce",
                FieldMaps: []FieldMapping{
                    {Field: "payment.state", Operator: "eq", Value: "Failed"},
                },
                Confidence: 0.95,
            },
        },
    }
}

func (e *LexicalExpander) Expand(input string) ExpandedQuery {
    lower := strings.ToLower(input)
    tokens := strings.Fields(lower)

    // Step 1: Expand abbreviations
    expandedTokens := make([]string, len(tokens))
    for i, tok := range tokens {
        if expanded, ok := e.abbreviations[tok]; ok {
            expandedTokens[i] = expanded
        } else {
            expandedTokens[i] = tok
        }
    }

    // Step 2: Map domain terms to fields
    normalized := strings.Join(expandedTokens, " ")

    // Step 3: Generate synonym variants
    variants := []string{normalized}
    synonyms := make(map[string][]string)
    for i, tok := range expandedTokens {
        if syns, ok := e.synonymMap[tok]; ok {
            synonyms[tok] = syns
            for _, syn := range syns {
                variant := make([]string, len(expandedTokens))
                copy(variant, expandedTokens)
                variant[i] = syn
                variants = append(variants, strings.Join(variant, " "))
            }
        }
    }

    return ExpandedQuery{
        Original:      input,
        Normalized:    normalized,
        Expansions:    variants,
        Synonyms:      synonyms,
        InferredTerms: expandedTokens,
        Confidence:    0.85,
    }
}

func (e *LexicalExpander) MapToFilters(input string) []store.Filter {
    lower := strings.ToLower(input)
    filters := []store.Filter{}

    // Match domain terms
    for term, domainTerm := range e.domainTerms {
        if strings.Contains(lower, term) {
            for _, mapping := range domainTerm.FieldMaps {
                filters = append(filters, store.Filter{
                    Field: mapping.Field,
                    Op:    mapping.Operator,
                    Value: resolveValue(mapping.Value),
                })
            }
        }
    }

    return filters
}

func resolveValue(v interface{}) interface{} {
    if str, ok := v.(string); ok {
        if strings.HasPrefix(str, "now-") {
            // Resolve relative time: "now-7d" -> time.Now().AddDate(0, 0, -7)
            duration := str[4:] // Extract "7d"
            if strings.HasSuffix(duration, "d") {
                days, _ := strconv.Atoi(duration[:len(duration)-1])
                return time.Now().AddDate(0, 0, -days).Format(time.RFC3339)
            }
        }
    }
    return v
}
```

### 2. Semantic Query Expansion with Embeddings

```go
// /internal/semantic/semantic_expander.go

type SemanticExpander struct {
    embeddingProvider EmbeddingProvider
    expansionIndex    *ExpansionIndex
}

type ExpansionIndex struct {
    termEmbeddings   map[string][]float64
    queryTemplates   []QueryTemplate
}

type QueryTemplate struct {
    Pattern     string
    Embeddings  []float64
    Filters     []store.Filter
    Intent      string
    Confidence  float64
}

func (se *SemanticExpander) ExpandSemantically(input string) []QueryTemplate {
    // Embed the input query
    queryEmb, err := se.embeddingProvider.Embed(input)
    if err != nil {
        return nil
    }

    // Find similar query templates
    matches := []QueryTemplate{}
    for _, template := range se.expansionIndex.queryTemplates {
        similarity := cosineSimilarity(queryEmb, template.Embeddings)
        if similarity > 0.75 {
            template.Confidence = similarity
            matches = append(matches, template)
        }
    }

    // Sort by similarity
    sort.Slice(matches, func(i, j int) bool {
        return matches[i].Confidence > matches[j].Confidence
    })

    return matches
}

// Pre-built query templates (these can be learned from usage logs)
var queryTemplates = []QueryTemplate{
    {
        Pattern: "order delayed",
        Filters: []store.Filter{
            {Field: "shipment.state", Op: "eq", Value: "Delayed"},
        },
        Intent: "wismo",
        Confidence: 0.9,
    },
    {
        Pattern: "package not received",
        Filters: []store.Filter{
            {Field: "shipment.state", Op: "neq", Value: "Delivered"},
            {Field: "order.created_at", Op: "lt", Value: "now-7d"},
        },
        Intent: "wismo",
        Confidence: 0.88,
    },
    {
        Pattern: "vip customer issue",
        Filters: []store.Filter{
            {Field: "customer.vip_tier", Op: "in", Value: []string{"gold", "platinum", "diamond"}},
        },
        Intent: "crm_profile",
        Confidence: 0.85,
    },
}
```

### 3. Context-Aware Auto-Completion

```go
// /internal/semantic/autocomplete.go

type AutoCompleter struct {
    queryHistory  []string
    popularQueries map[string]int
    userQueries   map[string][]string  // userID -> recent queries
}

func (ac *AutoCompleter) Suggest(input string, userID string, limit int) []Suggestion {
    input = strings.ToLower(strings.TrimSpace(input))

    suggestions := []Suggestion{}

    // Strategy 1: Prefix match from query history
    for _, q := range ac.queryHistory {
        if strings.HasPrefix(strings.ToLower(q), input) {
            score := ac.popularQueries[q]
            suggestions = append(suggestions, Suggestion{
                Query:      q,
                Score:      float64(score),
                Source:     "history",
            })
        }
    }

    // Strategy 2: User's recent queries (personalized)
    if userQueries, ok := ac.userQueries[userID]; ok {
        for _, q := range userQueries {
            if strings.Contains(strings.ToLower(q), input) {
                suggestions = append(suggestions, Suggestion{
                    Query:  q,
                    Score:  100.0,  // High score for user's own queries
                    Source: "personal",
                })
            }
        }
    }

    // Strategy 3: Common completions for abbreviations
    completions := getCommonCompletions(input)
    for _, c := range completions {
        suggestions = append(suggestions, Suggestion{
            Query:  c,
            Score:  50.0,
            Source: "common",
        })
    }

    // Dedupe and sort by score
    suggestions = dedupeSuggestions(suggestions)
    sort.Slice(suggestions, func(i, j int) bool {
        return suggestions[i].Score > suggestions[j].Score
    })

    if len(suggestions) > limit {
        suggestions = suggestions[:limit]
    }

    return suggestions
}

type Suggestion struct {
    Query      string
    Score      float64
    Source     string
    Confidence float64
}

func getCommonCompletions(input string) []string {
    completionMap := map[string][]string{
        "ord":  {"order delayed", "order not shipped", "order status", "orders this week"},
        "cust": {"customer orders", "customer profile", "customer vip"},
        "pkg":  {"package tracking", "package delayed", "package not received"},
        "del":  {"delayed orders", "delivered orders"},
        "vip":  {"vip customers", "vip orders"},
    }

    if completions, ok := completionMap[input]; ok {
        return completions
    }

    return []string{}
}
```

### 4. Feedback-Based Query Refinement

```go
// /internal/semantic/query_refiner.go

type QueryRefiner struct {
    feedbackStore FeedbackStore
}

type FeedbackStore interface {
    RecordRefinement(original, refined string, success bool)
    GetRefinements(query string) []Refinement
}

type Refinement struct {
    OriginalQuery string
    RefinedQuery  string
    SuccessRate   float64
    UsageCount    int
}

func (qr *QueryRefiner) RefineBasedOnFeedback(input string) []string {
    // Look up historical refinements
    refinements := qr.feedbackStore.GetRefinements(input)

    // Return top refinements by success rate
    sort.Slice(refinements, func(i, j int) bool {
        return refinements[i].SuccessRate > refinements[j].SuccessRate
    })

    results := []string{}
    for _, r := range refinements {
        if r.SuccessRate > 0.7 && r.UsageCount > 5 {
            results = append(results, r.RefinedQuery)
        }
    }

    return results
}

// Example: User searches "ord delayed", gets no results, refines to "orders with shipment delayed"
// System learns: "ord delayed" -> "orders with shipment delayed" (success=true)
// Next time "ord delayed" is searched, auto-suggest the refined version
```

## Implementation Priority

### Phase 1 (Week 1-2): Lexical Expansion
- [ ] Build abbreviation dictionary (100+ terms)
- [ ] Build synonym map (50+ terms with 3-5 synonyms each)
- [ ] Build domain term -> filter mappings (30+ terms)
- [ ] Integrate with `ParseNaturalLanguage()`

### Phase 2 (Week 3-4): Query Templates
- [ ] Extract 100+ query templates from logs
- [ ] Generate embeddings for each template
- [ ] Build similarity search index
- [ ] Test template matching accuracy

### Phase 3 (Week 5): Auto-Completion
- [ ] Implement prefix-based autocomplete
- [ ] Add personalized suggestions
- [ ] Expose `/api/query/autocomplete` endpoint
- [ ] Integrate in UI

### Phase 4 (Week 6): Feedback Loop
- [ ] Add feedback tracking to query results
- [ ] Build refinement store (SQLite table)
- [ ] Implement learning from successful refinements
- [ ] Add "Did you mean?" suggestions

## Testing Strategy

### Test Cases

```go
func TestSparseQueryExpansion(t *testing.T) {
    expander := NewLexicalExpander()

    tests := []struct {
        input    string
        expected ExpandedQuery
    }{
        {
            input: "ord delayed",
            expected: ExpandedQuery{
                Original:   "ord delayed",
                Normalized: "order delayed",
                Expansions: []string{"order delayed", "order late", "purchase delayed"},
                InferredTerms: []string{"order", "delayed"},
            },
        },
        {
            input: "cust vip",
            expected: ExpandedQuery{
                Original:   "cust vip",
                Normalized: "customer vip",
                Expansions: []string{"customer vip", "customer premium", "client vip"},
            },
        },
        {
            input: "pkg not rcvd",
            expected: ExpandedQuery{
                Original:   "pkg not rcvd",
                Normalized: "package not received",
                Expansions: []string{"package not received", "order not received", "parcel not received"},
            },
        },
    }

    for _, tt := range tests {
        result := expander.Expand(tt.input)
        // Assert expansions match
    }
}

func TestDomainTermMapping(t *testing.T) {
    expander := NewLexicalExpander()

    tests := []struct {
        input           string
        expectedFilters []store.Filter
    }{
        {
            input: "delayed orders",
            expectedFilters: []store.Filter{
                {Field: "shipment.state", Op: "eq", Value: "Delayed"},
            },
        },
        {
            input: "vip customers",
            expectedFilters: []store.Filter{
                {Field: "customer.vip_tier", Op: "in", Value: []string{"gold", "platinum", "diamond"}},
            },
        },
        {
            input: "payment bounce",
            expectedFilters: []store.Filter{
                {Field: "payment.state", Op: "eq", Value: "Failed"},
            },
        },
    }

    for _, tt := range tests {
        result := expander.MapToFilters(tt.input)
        // Assert filters match
    }
}
```

## Expected Impact

| Query Type | Current Handling | After Enhancement |
|------------|------------------|-------------------|
| "ord delayed" | ❌ No results | ✅ Maps to shipment.state filters |
| "cust vip issue" | ❌ Misclassified | ✅ Expands to VIP filter + recent tickets |
| "pkg not rcvd" | ❌ No understanding | ✅ Maps to not delivered + time window |
| "stuck proc" | ❌ No match | ✅ Maps to order.state=Open + old created_at |
| Partial "ord d..." | ❌ No suggestions | ✅ Auto-completes to "order delayed" |

## Success Metrics

| Metric | Current | Target |
|--------|---------|--------|
| Abbreviation recognition | 0% | >95% |
| Sparse query handling | ~20% | >85% |
| Query expansion accuracy | N/A | >80% |
| Auto-complete relevance | N/A | >90% |
| User refinement rate | High (~40%) | Low (<15%) |
