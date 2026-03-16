# Query Rewriting & SLM Enhancement Plan

## Problem Statement
Current SLM prompt (`analyzer_slm_local.go:311-323`) is too generic and doesn't provide sufficient context for accurate query rewriting. The SLM often generates invalid filters or misses intent nuances.

## Current Issues

### 1. Weak Prompting Strategy
```go
// Current prompt (analyzer_slm_local.go:311)
func buildRewritePrompt(message, contractVersion, resourceHint string) string {
    return "You are a rewrite engine. Return only JSON with fields intent,intentCategory,resourceType,confidence,clarificationNeeded,safeEvidence,query. " +
        "Query must include contractVersion,intentCategory,filters,sort,page and only allowed field names. " +
        "contractVersion=" + contractVersion + ", resourceHint=" + resourceHint + ", message=" + message
}
```

**Problems:**
- ❌ No schema provided (SLM doesn't know valid fields)
- ❌ No examples (no few-shot learning)
- ❌ No domain knowledge (order states, payment states not explained)
- ❌ No operator constraints (which ops are valid per field type)
- ❌ No validation rules (intent-field compatibility)

### 2. No Repair Loop Sophistication
```go
// Current repair prompt is also weak (analyzer_slm_local.go:317)
func buildRewriteRepairPrompt(...) string {
    return "You are a rewrite repair engine. Fix JSON output..." +
        "validationErrors=" + strings.Join(validationErrors, "|")
}
```

## Proposed Solution: Structured Prompt Engineering

### 1. Schema-Aware Prompting

```go
// /internal/semantic/prompts.go

type PromptBuilder struct {
    schemaProvider  SchemaProvider
    exampleProvider ExampleProvider
    contractVersion string
}

func (pb *PromptBuilder) BuildRewritePrompt(req AnalyzeRequest) string {
    schema := pb.schemaProvider.GetSchema(req.ResourceHint, req.ContractVersion)
    examples := pb.exampleProvider.GetExamples(req.IntentHint)

    prompt := fmt.Sprintf(`You are an expert query rewriter for an e-commerce support search system.

**Task**: Convert natural language queries from support agents into structured search filters.

**Input Query**: %s
**Resource Type**: %s (order | customer)
**Contract Version**: %s

---

## Field Schema for %s

%s

---

## Valid Operators

- **Equality**: eq, neq
- **Comparison**: gt, gte, lt, lte
- **Text search**: like (for partial string matching)
- **Set membership**: in (for array of values)

---

## Intent Categories

1. **wismo** (Where Is My Order)
   - Focus: shipment tracking, delivery status, order timeline
   - Allowed fields: order.number, shipment.*, order.state, payment.state, tracking_id
   - Common filters: shipment.state, order.created_at

2. **crm_profile** (Customer Relationship Management)
   - Focus: customer history, profile data, segmentation
   - Allowed fields: customer.*, order.customer_email, order.customer_id
   - Common filters: customer.vip_tier, customer.email, customer.created_at

3. **returns_refunds** (Returns & Refunds)
   - Focus: return eligibility, refund status
   - Allowed fields: return.*, refund.*, order.number
   - Common filters: return.status, refund.status, return.eligible

---

## Few-Shot Examples

%s

---

## Output Format

Return ONLY valid JSON with this exact structure (no markdown, no explanation):

{
  "intent": "search_order",
  "intentCategory": "wismo",
  "intentSubcategory": "shipping_tracking",
  "resourceType": "order",
  "confidence": 0.85,
  "clarificationNeeded": false,
  "safeEvidence": ["tracking_id_lookup", "wismo:tracking"],
  "query": {
    "contractVersion": "%s",
    "intentCategory": "wismo",
    "filters": [
      {"field": "shipment.tracking_id", "op": "eq", "value": "TRK-12345678"}
    ],
    "sort": {"field": "order.created_at", "dir": "desc"},
    "page": {"limit": 20, "offset": 0}
  }
}

**Critical Rules:**
1. Only use fields allowed for the intent category
2. Use correct operators for field types (eq for IDs, like for partial text)
3. Normalize all identifiers (ORD-XXXXXX, TRK-XXXXXXXX, CUST-XXXXXX)
4. Set clarificationNeeded=true if query is ambiguous
5. Include safeEvidence array with reason codes for filter choices

Now rewrite the input query:`,
        req.Message,
        req.ResourceHint,
        req.ContractVersion,
        req.ResourceHint,
        schema.RenderFieldList(),
        examples.Render(),
        req.ContractVersion,
    )

    return prompt
}
```

### 2. Dynamic Schema Provider

```go
// /internal/semantic/schema_provider.go

type SchemaProvider interface {
    GetSchema(resourceType, contractVersion string) *ResourceSchema
}

type ResourceSchema struct {
    ResourceType string
    Fields       []FieldDef
}

type FieldDef struct {
    Name         string
    Type         string   // string, int, float, timestamp, enum
    Description  string
    EnumValues   []string // For enum types
    Operators    []string // Allowed operators for this field
    IntentScopes []string // Which intents can access this field
}

func (rs *ResourceSchema) RenderFieldList() string {
    var b strings.Builder
    b.WriteString("| Field Name | Type | Allowed Operators | Intent Scope | Description |\n")
    b.WriteString("|------------|------|-------------------|--------------|-------------|\n")
    for _, f := range rs.Fields {
        b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
            f.Name,
            f.Type,
            strings.Join(f.Operators, ", "),
            strings.Join(f.IntentScopes, ", "),
            f.Description,
        ))
    }
    return b.String()
}

func NewContractV2SchemaProvider() *ContractV2SchemaProvider {
    return &ContractV2SchemaProvider{
        schemas: map[string]*ResourceSchema{
            "order": {
                ResourceType: "order",
                Fields: []FieldDef{
                    {
                        Name:         "order.number",
                        Type:         "string",
                        Description:  "Order identifier (format: ORD-XXXXXX)",
                        EnumValues:   nil,
                        Operators:    []string{"eq", "like"},
                        IntentScopes: []string{"wismo", "crm_profile", "returns_refunds"},
                    },
                    {
                        Name:         "order.state",
                        Type:         "enum",
                        Description:  "Current order state",
                        EnumValues:   []string{"Open", "Confirmed", "Complete", "Cancelled"},
                        Operators:    []string{"eq", "neq", "in"},
                        IntentScopes: []string{"wismo", "crm_profile"},
                    },
                    {
                        Name:         "shipment.state",
                        Type:         "enum",
                        Description:  "Current shipment status",
                        EnumValues:   []string{"Pending", "Shipped", "Delivered", "Delayed", "Ready"},
                        Operators:    []string{"eq", "neq", "in"},
                        IntentScopes: []string{"wismo"},
                    },
                    {
                        Name:         "shipment.tracking_id",
                        Type:         "string",
                        Description:  "Carrier tracking number (format: TRK-XXXXXXXX)",
                        EnumValues:   nil,
                        Operators:    []string{"eq", "like"},
                        IntentScopes: []string{"wismo"},
                    },
                    {
                        Name:         "payment.state",
                        Type:         "enum",
                        Description:  "Payment status",
                        EnumValues:   []string{"Pending", "Paid", "Failed", "Refunded"},
                        Operators:    []string{"eq", "neq", "in"},
                        IntentScopes: []string{"wismo", "crm_profile"},
                    },
                    {
                        Name:         "payment.reference",
                        Type:         "string",
                        Description:  "Payment transaction reference (format: PAY-XXXXXXXX)",
                        EnumValues:   nil,
                        Operators:    []string{"eq"},
                        IntentScopes: []string{"wismo"},
                    },
                    {
                        Name:         "order.customer_email",
                        Type:         "string",
                        Description:  "Customer email address",
                        EnumValues:   nil,
                        Operators:    []string{"eq", "like"},
                        IntentScopes: []string{"wismo", "crm_profile"},
                    },
                    {
                        Name:         "order.customer_id",
                        Type:         "string",
                        Description:  "Internal customer ID (format: cust-XXXXX)",
                        EnumValues:   nil,
                        Operators:    []string{"eq"},
                        IntentScopes: []string{"wismo", "crm_profile"},
                    },
                    {
                        Name:         "order.created_at",
                        Type:         "timestamp",
                        Description:  "Order creation timestamp (ISO8601 format)",
                        EnumValues:   nil,
                        Operators:    []string{"gt", "gte", "lt", "lte"},
                        IntentScopes: []string{"wismo", "crm_profile", "returns_refunds"},
                    },
                    {
                        Name:         "return.status",
                        Type:         "enum",
                        Description:  "Return request status",
                        EnumValues:   []string{"NotRequested", "Requested", "Approved", "Completed"},
                        Operators:    []string{"eq", "neq"},
                        IntentScopes: []string{"returns_refunds"},
                    },
                    {
                        Name:         "return.eligible",
                        Type:         "string",
                        Description:  "Return eligibility flag",
                        EnumValues:   []string{"true", "false"},
                        Operators:    []string{"eq"},
                        IntentScopes: []string{"returns_refunds"},
                    },
                    {
                        Name:         "refund.status",
                        Type:         "enum",
                        Description:  "Refund processing status",
                        EnumValues:   []string{"NotInitiated", "Pending", "Processed", "Failed"},
                        Operators:    []string{"eq", "neq"},
                        IntentScopes: []string{"returns_refunds"},
                    },
                },
            },
            "customer": {
                ResourceType: "customer",
                Fields: []FieldDef{
                    {
                        Name:         "customer.number",
                        Type:         "string",
                        Description:  "Customer identifier (format: CUST-XXXXXX)",
                        EnumValues:   nil,
                        Operators:    []string{"eq", "like"},
                        IntentScopes: []string{"crm_profile"},
                    },
                    {
                        Name:         "customer.email",
                        Type:         "string",
                        Description:  "Customer email address",
                        EnumValues:   nil,
                        Operators:    []string{"eq", "like"},
                        IntentScopes: []string{"crm_profile"},
                    },
                    {
                        Name:         "customer.vip_tier",
                        Type:         "enum",
                        Description:  "VIP loyalty tier",
                        EnumValues:   []string{"gold", "platinum", "diamond", "silver"},
                        Operators:    []string{"eq", "neq", "in"},
                        IntentScopes: []string{"crm_profile"},
                    },
                    {
                        Name:         "customer.customer_group",
                        Type:         "string",
                        Description:  "Customer segment group",
                        EnumValues:   nil,
                        Operators:    []string{"eq", "like"},
                        IntentScopes: []string{"crm_profile"},
                    },
                    {
                        Name:         "customer.is_email_verified",
                        Type:         "int",
                        Description:  "Email verification status (1=verified, 0=unverified)",
                        EnumValues:   nil,
                        Operators:    []string{"eq"},
                        IntentScopes: []string{"crm_profile"},
                    },
                    {
                        Name:         "customer.created_at",
                        Type:         "timestamp",
                        Description:  "Customer account creation timestamp",
                        EnumValues:   nil,
                        Operators:    []string{"gt", "gte", "lt", "lte"},
                        IntentScopes: []string{"crm_profile"},
                    },
                },
            },
        },
    }
}
```

### 3. Few-Shot Example Provider

```go
// /internal/semantic/example_provider.go

type ExampleProvider interface {
    GetExamples(intentHint string) *ExampleSet
}

type ExampleSet struct {
    Examples []Example
}

type Example struct {
    Query    string
    Output   AnalyzeResult
}

func (es *ExampleSet) Render() string {
    var b strings.Builder
    for i, ex := range es.Examples {
        b.WriteString(fmt.Sprintf("\n### Example %d\n", i+1))
        b.WriteString(fmt.Sprintf("**Input**: \"%s\"\n\n", ex.Query))
        b.WriteString("**Output**:\n```json\n")
        jsonBytes, _ := json.MarshalIndent(ex.Output, "", "  ")
        b.WriteString(string(jsonBytes))
        b.WriteString("\n```\n")
    }
    return b.String()
}

func NewStaticExampleProvider() *StaticExampleProvider {
    return &StaticExampleProvider{
        examples: map[string][]Example{
            "wismo": {
                {
                    Query: "where is order ORD-123456",
                    Output: AnalyzeResult{
                        Intent:            "search_order",
                        IntentCategory:    "wismo",
                        IntentSubcategory: "shipping_tracking",
                        ResourceType:      "order",
                        Confidence:        0.95,
                        ClarificationNeeded: false,
                        SafeEvidence:      []string{"order_number_lookup", "wismo:tracking"},
                        Query: store.QueryDSL{
                            ContractVersion: "v2",
                            IntentCategory:  "wismo",
                            Filters: []store.Filter{
                                {Field: "order.number", Op: "eq", Value: "ORD-123456"},
                            },
                            Sort: store.Sort{Field: "order.created_at", Dir: "desc"},
                            Page: store.Page{Limit: 20, Offset: 0},
                        },
                    },
                },
                {
                    Query: "show open orders with failed payment",
                    Output: AnalyzeResult{
                        Intent:            "search_order",
                        IntentCategory:    "wismo",
                        IntentSubcategory: "",
                        ResourceType:      "order",
                        Confidence:        0.9,
                        ClarificationNeeded: false,
                        SafeEvidence:      []string{"order_state:Open", "payment_state:Failed"},
                        Query: store.QueryDSL{
                            ContractVersion: "v2",
                            IntentCategory:  "wismo",
                            Filters: []store.Filter{
                                {Field: "order.state", Op: "eq", Value: "Open"},
                                {Field: "payment.state", Op: "eq", Value: "Failed"},
                            },
                            Sort: store.Sort{Field: "order.created_at", Dir: "desc"},
                            Page: store.Page{Limit: 20, Offset: 0},
                        },
                    },
                },
                {
                    Query: "orders not shipped this week",
                    Output: AnalyzeResult{
                        Intent:            "search_order",
                        IntentCategory:    "wismo",
                        IntentSubcategory: "shipping_tracking",
                        ResourceType:      "order",
                        Confidence:        0.88,
                        ClarificationNeeded: false,
                        SafeEvidence:      []string{"shipment_state:NOT_Shipped", "time_window:week"},
                        Query: store.QueryDSL{
                            ContractVersion: "v2",
                            IntentCategory:  "wismo",
                            Filters: []store.Filter{
                                {Field: "shipment.state", Op: "neq", Value: "Shipped"},
                                {Field: "shipment.state", Op: "neq", Value: "Delivered"},
                                {Field: "order.created_at", Op: "gte", Value: "2025-03-07T00:00:00Z"},
                            },
                            Sort: store.Sort{Field: "order.created_at", Dir: "desc"},
                            Page: store.Page{Limit: 20, Offset: 0},
                        },
                    },
                },
            },
            "crm_profile": {
                {
                    Query: "orders for customer aster@example.com",
                    Output: AnalyzeResult{
                        Intent:            "search_order",
                        IntentCategory:    "crm_profile",
                        ResourceType:      "order",
                        Confidence:        0.92,
                        ClarificationNeeded: false,
                        SafeEvidence:      []string{"customer_email_lookup"},
                        Query: store.QueryDSL{
                            ContractVersion: "v2",
                            IntentCategory:  "crm_profile",
                            Filters: []store.Filter{
                                {Field: "order.customer_email", Op: "eq", Value: "aster@example.com"},
                            },
                            Sort: store.Sort{Field: "order.created_at", Dir: "desc"},
                            Page: store.Page{Limit: 20, Offset: 0},
                        },
                    },
                },
                {
                    Query: "VIP customers",
                    Output: AnalyzeResult{
                        Intent:            "search_customer",
                        IntentCategory:    "crm_profile",
                        ResourceType:      "customer",
                        Confidence:        0.85,
                        ClarificationNeeded: false,
                        SafeEvidence:      []string{"vip_profile_filter"},
                        Query: store.QueryDSL{
                            ContractVersion: "v2",
                            IntentCategory:  "crm_profile",
                            Filters: []store.Filter{
                                {Field: "customer.vip_tier", Op: "neq", Value: "silver"},
                            },
                            Sort: store.Sort{Field: "customer.created_at", Dir: "desc"},
                            Page: store.Page{Limit: 20, Offset: 0},
                        },
                    },
                },
            },
            "returns_refunds": {
                {
                    Query: "orders eligible for return",
                    Output: AnalyzeResult{
                        Intent:            "search_order",
                        IntentCategory:    "returns_refunds",
                        ResourceType:      "order",
                        Confidence:        0.9,
                        ClarificationNeeded: false,
                        SafeEvidence:      []string{"return_eligible:true"},
                        Query: store.QueryDSL{
                            ContractVersion: "v2",
                            IntentCategory:  "returns_refunds",
                            Filters: []store.Filter{
                                {Field: "return.eligible", Op: "eq", Value: "true"},
                            },
                            Sort: store.Sort{Field: "order.created_at", Dir: "desc"},
                            Page: store.Page{Limit: 20, Offset: 0},
                        },
                    },
                },
            },
        },
    }
}
```

### 4. Intelligent Repair Loop

```go
// /internal/semantic/repair_engine.go

func (a *SLMLocalAnalyzer) tryRepairRemoteSLMEnhanced(
    req AnalyzeRequest,
    previous map[string]interface{},
    validationErrors []string,
) (AnalyzeResult, bool) {

    // Categorize errors
    errorTypes := categorizeValidationErrors(validationErrors)

    // Build targeted repair prompt based on error types
    repairPrompt := buildTargetedRepairPrompt(req, previous, errorTypes)

    // Make repair call
    result, ok := a.callOllamaWithPrompt(repairPrompt)
    if !ok {
        return AnalyzeResult{}, false
    }

    return result, true
}

func categorizeValidationErrors(errors []string) map[string][]string {
    categories := map[string][]string{
        "field_not_allowed":   {},
        "invalid_operator":    {},
        "invalid_value":       {},
        "intent_mismatch":     {},
        "resource_mismatch":   {},
    }

    for _, err := range errors {
        if strings.Contains(err, "field") && strings.Contains(err, "not allowed") {
            categories["field_not_allowed"] = append(categories["field_not_allowed"], err)
        } else if strings.Contains(err, "operator") {
            categories["invalid_operator"] = append(categories["invalid_operator"], err)
        } else if strings.Contains(err, "intent") {
            categories["intent_mismatch"] = append(categories["intent_mismatch"], err)
        }
        // ... other categorizations
    }

    return categories
}

func buildTargetedRepairPrompt(
    req AnalyzeRequest,
    previous map[string]interface{},
    errorTypes map[string][]string,
) string {

    prompt := "Your previous query rewrite had validation errors. Fix them as follows:\n\n"

    if len(errorTypes["field_not_allowed"]) > 0 {
        prompt += "**Field Not Allowed Errors:**\n"
        for _, err := range errorTypes["field_not_allowed"] {
            prompt += fmt.Sprintf("- %s\n", err)
        }
        prompt += "\n**Fix**: Remove these fields or change intentCategory to allow them.\n\n"
    }

    if len(errorTypes["invalid_operator"]) > 0 {
        prompt += "**Invalid Operator Errors:**\n"
        for _, err := range errorTypes["invalid_operator"] {
            prompt += fmt.Sprintf("- %s\n", err)
        }
        prompt += "\n**Fix**: Use only: eq, neq, gt, gte, lt, lte, like, in\n\n"
    }

    prompt += fmt.Sprintf("\n**Original Query**: %s\n", req.Message)
    prompt += fmt.Sprintf("\n**Previous Output (INVALID)**:\n```json\n%s\n```\n\n", toJSONString(previous))
    prompt += "Return corrected JSON (no markdown, no explanation):"

    return prompt
}
```

## Implementation Plan

### Week 1: Schema Provider
- [ ] Implement `ResourceSchema` and `FieldDef` structs
- [ ] Build schema for orders (v2 contract)
- [ ] Build schema for customers (v2 contract)
- [ ] Add schema rendering to markdown table format
- [ ] Write tests for schema validation

### Week 2: Example Provider
- [ ] Create 10+ examples per intent category
- [ ] Implement example rendering
- [ ] Add example selection based on query similarity
- [ ] Test few-shot prompt quality

### Week 3: Enhanced Prompt Builder
- [ ] Implement `PromptBuilder` with schema + examples
- [ ] Replace `buildRewritePrompt()` with enhanced version
- [ ] A/B test enhanced vs current prompts
- [ ] Measure accuracy improvement

### Week 4: Repair Engine
- [ ] Implement error categorization
- [ ] Build targeted repair prompts
- [ ] Add repair attempt tracking (max 2 attempts)
- [ ] Validate repair success rate

## Success Metrics

| Metric | Current | Target |
|--------|---------|--------|
| SLM accuracy (valid JSON) | ~60% | >90% |
| Filter field correctness | ~70% | >95% |
| Intent classification | ~75% | >92% |
| Repair success rate | ~40% | >80% |
| Latency (p95) | ~1200ms | <800ms |

## Model Selection Recommendations

### Current: Llama 3.2 / Qwen 2.5
- Works for basic rewrites
- Struggles with complex schema adherence
- ~1.5s latency on CPU

### Recommended Upgrades:
1. **Llama 3.1 8B Instruct** (best balance)
   - Better instruction following
   - ~800ms latency with quantization
   - Higher schema compliance

2. **Qwen 2.5 7B Instruct**
   - Fast inference (~600ms)
   - Good at structured output
   - Strong few-shot learning

3. **Mistral 7B v0.3**
   - Excellent JSON generation
   - ~700ms latency
   - Robust to prompt variations

### Production: Consider API-based SLM
- **OpenAI GPT-4o-mini** (~200ms, $0.15/1M tokens)
- **Anthropic Claude Haiku** (~300ms, $0.25/1M tokens)
- **Google Gemini Flash** (~250ms, $0.075/1M tokens)
