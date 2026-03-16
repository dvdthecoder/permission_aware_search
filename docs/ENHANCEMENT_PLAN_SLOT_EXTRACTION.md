# Slot Extraction Enhancement Plan

## Problem Statement
Current slot extraction (`slots.go`) only extracts 5 basic slots using regex patterns. Support agents use rich, varied language that needs better entity extraction.

## Proposed Solution: NER-Enhanced Slot Extraction

### Current Limitations
```go
// slots.go only extracts:
type SemanticSlots struct {
    OrderState       string  // Only: Open
    PaymentState     string  // Only: Paid, Failed, Pending
    ShipmentState    string  // Only: Shipped, negation
    TimeWindowRecent bool
    HasOrderKeyword  bool
}
```

### Enhanced Slot Schema
```go
type EnhancedSemanticSlots struct {
    // Current slots (keep)
    OrderState          string
    PaymentState        string
    ShipmentState       string
    TimeWindowRecent    bool

    // NEW: Temporal slots
    TimeWindow          *TimeWindow  // {start, end, type: absolute|relative}
    CreatedBefore       *time.Time
    CreatedAfter        *time.Time
    UpdatedSince        *time.Time

    // NEW: Numeric slots
    AmountRange         *AmountRange  // {min, max, currency}
    Quantity            *int

    // NEW: Entity slots
    CarrierName         string        // UPS, FedEx, USPS, DHL
    ProductName         string        // From query context
    CustomerSegment     string        // VIP, premium, regular
    Region              string        // US, EU, APAC

    // NEW: Status reason slots
    DelayReason         string        // weather, customs, inventory
    RefundReason        string        // defective, wrong_item, cancelled
    ReturnReason        string

    // NEW: Comparison slots
    ComparisonOperator  string        // greater_than, less_than, between, equals
    ComparisonValue     interface{}

    // NEW: Negation tracking
    NegatedFields       []string      // Fields with NOT/negation

    // NEW: Relationship slots
    CustomerRelation    string        // same, different, specific
    OrderRelation       string        // previous, next, related

    // Metadata
    ExtractedEntities   map[string][]Entity
    SlotConfidence      map[string]float64
    AmbiguousSlots      []string
}

type TimeWindow struct {
    Start      time.Time
    End        time.Time
    Type       string  // absolute, relative, fuzzy
    Label      string  // "last week", "Q4 2024", "yesterday"
}

type AmountRange struct {
    Min      *float64
    Max      *float64
    Currency string
}

type Entity struct {
    Text       string
    Type       string  // CARRIER, PRODUCT, REGION, etc.
    Normalized string
    Confidence float64
}
```

## Implementation Strategy

### 1. Regex-Based Entity Extraction (Quick Wins)

```go
// /internal/semantic/entity_extractor.go

var (
    // Carrier patterns
    reCarrier = regexp.MustCompile(`(?i)\b(ups|fedex|usps|dhl|aramex)\b`)

    // Amount patterns
    reAmount = regexp.MustCompile(`(?i)\$([\d,]+(?:\.\d{2})?)|(\d+)\s*(dollars?|usd|eur|gbp)`)

    // Date patterns
    reDateAbsolute = regexp.MustCompile(`\d{1,2}[/-]\d{1,2}[/-]\d{2,4}`)
    reDateRelative = regexp.MustCompile(`(?i)(yesterday|today|tomorrow|last\s+week|this\s+month|next\s+\w+)`)

    // Region patterns
    reRegion = regexp.MustCompile(`(?i)\b(US|USA|EU|UK|APAC|Canada|Germany|France)\b`)

    // Status reason patterns
    reDelayReason = regexp.MustCompile(`(?i)\b(weather|customs?|inventory|warehouse|out\s+of\s+stock)\b`)
)

func extractEnhancedSlots(input string) EnhancedSemanticSlots {
    slots := EnhancedSemanticSlots{
        ExtractedEntities: make(map[string][]Entity),
        SlotConfidence:    make(map[string]float64),
        AmbiguousSlots:    []string{},
    }

    // Extract carriers
    if m := reCarrier.FindStringSubmatch(input); len(m) > 1 {
        carrier := normalizeCarrier(m[1])
        slots.CarrierName = carrier
        slots.ExtractedEntities["CARRIER"] = []Entity{
            {Text: m[1], Type: "CARRIER", Normalized: carrier, Confidence: 0.95},
        }
        slots.SlotConfidence["carrier"] = 0.95
    }

    // Extract amounts
    if amounts := extractAmounts(input); len(amounts) > 0 {
        slots.AmountRange = &AmountRange{
            Min:      amounts[0].Min,
            Max:      amounts[0].Max,
            Currency: amounts[0].Currency,
        }
        slots.SlotConfidence["amount"] = 0.9
    }

    // Extract temporal windows
    if tw := extractTimeWindow(input); tw != nil {
        slots.TimeWindow = tw
        slots.SlotConfidence["time_window"] = tw.Confidence
    }

    // Extract regions
    if regions := extractRegions(input); len(regions) > 0 {
        slots.Region = regions[0].Normalized
        slots.ExtractedEntities["REGION"] = regions
        slots.SlotConfidence["region"] = 0.85
    }

    // Extract delay reasons
    if reason := extractDelayReason(input); reason != "" {
        slots.DelayReason = reason
        slots.SlotConfidence["delay_reason"] = 0.8
    }

    return slots
}

func extractAmounts(input string) []AmountRange {
    var amounts []AmountRange
    matches := reAmount.FindAllStringSubmatch(input, -1)
    for _, m := range matches {
        if len(m) > 1 && m[1] != "" {
            // $100 format
            val := parseFloat(strings.ReplaceAll(m[1], ",", ""))
            amounts = append(amounts, AmountRange{Min: &val, Currency: "USD"})
        } else if len(m) > 2 && m[2] != "" {
            // 100 dollars format
            val := parseFloat(m[2])
            currency := normalizeCurrency(m[3])
            amounts = append(amounts, AmountRange{Min: &val, Currency: currency})
        }
    }
    return amounts
}

func extractTimeWindow(input string) *TimeWindow {
    lower := strings.ToLower(input)

    // Relative time windows
    relativePatterns := map[string]func() TimeWindow{
        "yesterday": func() TimeWindow {
            return TimeWindow{
                Start: time.Now().AddDate(0, 0, -1).Truncate(24 * time.Hour),
                End:   time.Now().Truncate(24 * time.Hour),
                Type:  "relative",
                Label: "yesterday",
            }
        },
        "last week": func() TimeWindow {
            return TimeWindow{
                Start: time.Now().AddDate(0, 0, -7),
                End:   time.Now(),
                Type:  "relative",
                Label: "last week",
            }
        },
        "this month": func() TimeWindow {
            now := time.Now()
            start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
            return TimeWindow{
                Start: start,
                End:   now,
                Type:  "relative",
                Label: "this month",
            }
        },
        "last 30 days": func() TimeWindow {
            return TimeWindow{
                Start: time.Now().AddDate(0, 0, -30),
                End:   time.Now(),
                Type:  "relative",
                Label: "last 30 days",
            }
        },
    }

    for pattern, builder := range relativePatterns {
        if strings.Contains(lower, pattern) {
            tw := builder()
            return &tw
        }
    }

    // Absolute dates (2024-12-01)
    if m := reDateAbsolute.FindString(input); m != "" {
        if t, err := parseDate(m); err == nil {
            return &TimeWindow{
                Start: t,
                End:   t.Add(24 * time.Hour),
                Type:  "absolute",
                Label: m,
            }
        }
    }

    return nil
}
```

### 2. Negation Detection

```go
// /internal/semantic/negation_detector.go

type NegationScope struct {
    NegationWord string   // "not", "no", "without", "except"
    ScopeStart   int      // Token index where negation starts
    ScopeEnd     int      // Token index where negation ends
    AffectedTerms []string
}

func detectNegations(input string) []NegationScope {
    tokens := tokenize(input)
    negations := []NegationScope{}

    negationTriggers := map[string]int{
        "not":     3,  // Scope: 3 tokens ahead
        "no":      2,
        "without": 2,
        "except":  3,
        "excluding": 3,
        "hasn't":  2,
        "haven't": 2,
        "won't":   2,
    }

    for i, token := range tokens {
        if scope, ok := negationTriggers[strings.ToLower(token)]; ok {
            endIdx := min(i+scope+1, len(tokens))
            negations = append(negations, NegationScope{
                NegationWord:  token,
                ScopeStart:    i,
                ScopeEnd:      endIdx,
                AffectedTerms: tokens[i+1 : endIdx],
            })
        }
    }

    return negations
}

// Usage in filter building:
func applyNegationToFilters(query *store.QueryDSL, negations []NegationScope, slots EnhancedSemanticSlots) {
    for _, neg := range negations {
        for _, term := range neg.AffectedTerms {
            // If negation affects "shipped", add shipment.state != Shipped
            if strings.Contains(strings.ToLower(term), "shipped") {
                query.Filters = append(query.Filters, store.Filter{
                    Field: "shipment.state",
                    Op:    "neq",
                    Value: "Shipped",
                })
            }
            // If negation affects "vip", add customer.vip_tier = silver
            if strings.Contains(strings.ToLower(term), "vip") {
                query.Filters = append(query.Filters, store.Filter{
                    Field: "customer.vip_tier",
                    Op:    "eq",
                    Value: "silver",
                })
            }
        }
    }
}
```

### 3. Compound Query Parsing

```go
// /internal/semantic/compound_query_parser.go

type QueryClause struct {
    Type       string   // AND, OR, NOT
    Conditions []Condition
}

type Condition struct {
    Field    string
    Operator string
    Value    interface{}
    Negated  bool
}

func parseCompoundQuery(input string) []QueryClause {
    // Split on conjunction keywords
    clauses := []QueryClause{}

    // Pattern: "X AND Y" or "X but Y" or "X except Y"
    andPattern := regexp.MustCompile(`(?i)\s+(and|but|with|that have)\s+`)
    orPattern := regexp.MustCompile(`(?i)\s+(or|alternatively)\s+`)

    parts := andPattern.Split(input, -1)
    if len(parts) > 1 {
        for _, part := range parts {
            conditions := parseCondition(part)
            clauses = append(clauses, QueryClause{
                Type:       "AND",
                Conditions: conditions,
            })
        }
    }

    return clauses
}

// Example: "open orders from VIP customers with failed payment"
// -> Clause 1: order.state = Open
// -> Clause 2: customer.vip_tier != silver
// -> Clause 3: payment.state = Failed
```

## Testing Strategy

### Test Cases for Enhanced Slots

```go
func TestEnhancedSlotExtraction(t *testing.T) {
    tests := []struct {
        input    string
        expected EnhancedSemanticSlots
    }{
        {
            input: "orders over $100 shipped via UPS last week",
            expected: EnhancedSemanticSlots{
                AmountRange: &AmountRange{Min: float64Ptr(100), Currency: "USD"},
                CarrierName: "UPS",
                TimeWindow:  &TimeWindow{Label: "last week", Type: "relative"},
            },
        },
        {
            input: "VIP customers from EU region",
            expected: EnhancedSemanticSlots{
                CustomerSegment: "VIP",
                Region:          "EU",
            },
        },
        {
            input: "orders delayed due to weather",
            expected: EnhancedSemanticSlots{
                ShipmentState: "Delayed",
                DelayReason:   "weather",
            },
        },
        {
            input: "not shipped orders",
            expected: EnhancedSemanticSlots{
                HasShipmentNegate: true,
                NegatedFields:     []string{"shipment.state"},
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.input, func(t *testing.T) {
            result := extractEnhancedSlots(tt.input)
            // Assert expected fields
        })
    }
}
```

## Implementation Priority

### Phase 1 (Week 1-2): Basic Entity Extraction
- [ ] Implement regex-based carrier extraction
- [ ] Implement amount range extraction
- [ ] Implement enhanced time window parsing
- [ ] Add region/geography extraction

### Phase 2 (Week 3-4): Negation Handling
- [ ] Build negation detection
- [ ] Scope negations to affected terms
- [ ] Apply negations to filter building

### Phase 3 (Week 5-6): Compound Queries
- [ ] Parse AND/OR conjunctions
- [ ] Support multi-condition queries
- [ ] Handle nested conditions

### Phase 4 (Week 7-8): NER Integration (Optional)
- [ ] Integrate spaCy or Hugging Face NER
- [ ] Train custom NER model for e-commerce domain
- [ ] Add entity disambiguation

## Expected Impact

| Capability | Before | After |
|------------|--------|-------|
| Extractable slots | 5 types | 20+ types |
| Carrier detection | None | 95%+ |
| Amount filtering | None | 90%+ |
| Negation handling | Basic | Advanced |
| Compound queries | None | 2-3 clauses |
| Entity types | 0 | 10+ |
