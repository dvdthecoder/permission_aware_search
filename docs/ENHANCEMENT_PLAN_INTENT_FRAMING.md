# Intent Framing Enhancement Plan

## Problem Statement
Current rule-based intent classifier (`parser.go:classifyIntent`) is too brittle and keyword-dependent.

## Proposed Solution: Hierarchical Intent Classification

### Stage 1: Domain Classification (10-20ms)
```
Input: "need pkg location"
Output: [order_domain: 0.85, logistics_domain: 0.75]
```

**Implementation:**
- Train a lightweight classifier (200-500 examples per domain)
- Domains: order_operations, customer_profile, returns_refunds, unsupported
- Use TF-IDF + Logistic Regression OR lightweight transformer (DistilBERT)
- Cache embeddings for common queries

### Stage 2: Intent Subcategory (Rule + Model Hybrid)
```
If domain = order_operations:
  - WISMO (tracking, shipment status)
  - payment_investigation
  - fulfillment_issue
  - order_history
```

**Rule Enhancements:**
```go
// Current: exact phrase matching
if containsAny(lower, "where is my order", "wismo") { return IntentWISMO }

// Enhanced: semantic pattern matching with variants
func classifyIntentEnhanced(lower string, tokens []string) IntentScore {
    scores := make(map[string]float64)

    // WISMO signals (weighted)
    scores["wismo"] += 0.0
    if hasLocationCue(tokens) { scores["wismo"] += 0.3 }  // location, where, whereabouts
    if hasTrackingCue(tokens) { scores["wismo"] += 0.4 }  // tracking, track, status
    if hasShipmentCue(tokens) { scores["wismo"] += 0.3 }  // shipped, package, delivery
    if hasOrderIdentifier(lower) { scores["wismo"] += 0.2 }  // order#, tracking#

    // CRM signals
    if hasCustomerIdentifier(lower) { scores["crm_profile"] += 0.3 }
    if hasProfileCue(tokens) { scores["crm_profile"] += 0.4 }  // history, profile, segment
    if hasRelationalCue(tokens) { scores["crm_profile"] += 0.3 }  // all, recent, past

    return normalizeScores(scores)
}

func hasLocationCue(tokens []string) bool {
    locationTerms := []string{"where", "location", "whereabouts", "locate", "find"}
    return hasAnyToken(tokens, locationTerms)
}
```

### Stage 3: Intent Validation with SLM
```
Prompt: "Given query '{query}' classified as '{intent}', validate if correct.
Return: {intent: wismo|crm_profile|returns_refunds, confidence: 0-1, reason: string}"
```

## Implementation Files

### New Files to Create:
1. `/internal/semantic/intent_classifier_ml.go` - ML-based classifier
2. `/internal/semantic/intent_signals.go` - Signal extraction functions
3. `/testdata/intent_training_data.json` - Training examples

### Files to Modify:
1. `/internal/semantic/parser.go` - Replace `classifyIntent()` with hierarchical approach
2. `/internal/semantic/intent_framer.go` - Add ML-backed framer option
3. `/internal/semantic/analyzer_slm_local.go` - Add intent validation mode

## Training Data Collection

### Intent Training Examples (500+ per intent):

**WISMO Intent:**
```json
[
  {"query": "where is my order", "intent": "wismo", "confidence": 1.0},
  {"query": "pkg location", "intent": "wismo", "confidence": 0.95},
  {"query": "track shipment", "intent": "wismo", "confidence": 1.0},
  {"query": "order not arrived", "intent": "wismo", "confidence": 0.9},
  {"query": "delivery status", "intent": "wismo", "confidence": 0.95},
  {"query": "tracking number not working", "intent": "wismo", "confidence": 0.9},
  {"query": "when will it arrive", "intent": "wismo", "confidence": 0.85}
]
```

**CRM Profile Intent:**
```json
[
  {"query": "customer order history", "intent": "crm_profile", "confidence": 1.0},
  {"query": "all orders by email", "intent": "crm_profile", "confidence": 0.95},
  {"query": "vip customer info", "intent": "crm_profile", "confidence": 1.0},
  {"query": "past purchases for customer", "intent": "crm_profile", "confidence": 0.9}
]
```

## Expected Improvements

| Metric | Current | Target |
|--------|---------|--------|
| Intent classification accuracy | ~75% | ~92% |
| Support for paraphrases | Low | High |
| Multilingual queries | None | Basic (EN variants) |
| Confidence calibration | Static | Dynamic |
| Ambiguity detection | Manual | Automatic |

## Success Criteria
- [ ] Intent accuracy > 90% on golden test set
- [ ] Handles 20+ paraphrase variants per intent
- [ ] Sub-50ms latency for intent classification
- [ ] Confidence scores correlate with actual accuracy
