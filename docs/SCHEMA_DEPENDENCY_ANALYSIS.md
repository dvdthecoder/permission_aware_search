# Schema Dependency Analysis

## Executive Summary

**Verdict**: The codebase is **heavily dependent on the demo data schema** (order/customer structure). Approximately **70-80% of the core logic** assumes the specific field names, enum values, and resource types from the demo schema.

**To support a different schema would require**:
- ✏️ Modifying **~15 source files** (~1500+ lines of code)
- 📝 Rewriting **database migrations** and **seed data**
- 🔧 Updating **config files** for identifier patterns
- 🧪 Updating **all integration tests** with new test data

---

## Dependency Breakdown by Category

### 1. CRITICAL HARDCODED (Must Change for Different Schema)

#### Field Names - **100% Hardcoded**
**Impact**: HIGH | **Files Affected**: 8+

Hardcoded in multiple layers:

**Contract Layer** (`internal/contracts/fields.go`)
```go
// Order fields (30+ field definitions)
"order.number", "order.customer_id", "order.state", "order.created_at"
"shipment.state", "shipment.tracking_id"
"payment.state", "payment.reference"
"return.eligible", "return.status", "refund.status"

// Customer fields (11+ field definitions)
"customer.number", "customer.email", "customer.vip_tier"
"customer.first_name", "customer.last_name"
```

**Storage Layer** (`internal/store/sqlite/adapter.go`)
```go
func nativeOrderField(logicalField string) (string, bool) {
    switch logicalField {
    case "order.number":        return "orderNumber", true
    case "order.state":         return "orderState", true
    case "shipment.state":      return "shipmentState", true
    case "payment.reference":   return "paymentReference", true
    // ... 20+ more mappings
    }
}
```

**Schema Provider** (`internal/semantic/schema_provider.go`)
- All 254 lines define demo schema fields with types, operators, enum values

**Files to Change**:
- `internal/contracts/fields.go` (73 lines)
- `internal/contracts/contracts.go` (100+ lines for allowlists)
- `internal/store/sqlite/adapter.go` (267-350: field mappings)
- `internal/semantic/schema_provider.go` (entire file: 254 lines)

---

#### Resource Types - **100% Hardcoded**
**Impact**: HIGH | **Files Affected**: 10+

**Switch Statements on "order" and "customer"** (20+ occurrences):

```go
// Pattern repeated throughout codebase:
switch resourceType {
case "order":
    return "orders_docs", nil
case "customer":
    return "customers_docs", nil
default:
    return "", fmt.Errorf("unknown resource")
}
```

**Locations**:
- `internal/store/sqlite/adapter.go` (lines 149-157, 178-186)
- `internal/identifier/resolver.go` (lines 74-84: resourceHint filtering)
- `internal/semantic/parser.go` (lines 228-242: inferResourceType)
- `internal/http/server.go` (multiple resource type checks)

**Table Names Hardcoded**:
- `orders_docs` - migration, adapter, tests
- `customers_docs` - migration, adapter, tests

**Files to Change**:
- All files with switch statements on resourceType
- Database migrations (table names)
- All integration tests

---

#### Enum Values - **100% Hardcoded**
**Impact**: HIGH | **Files Affected**: 5+

**Order States**:
```go
// internal/semantic/schema_provider.go:75
EnumValues: []string{"Open", "Confirmed", "Complete", "Cancelled"}

// internal/semantic/parser.go:249
if slots.OrderStatusOpen {
    filters = append(filters, store.Filter{Field: "order.state", Op: "eq", Value: "Open"})
}
```

**Shipment States**:
```go
// Used in WISMO intent filtering
deny := []string{"Shipped", "Delivered", "Ready"}
for _, state := range deny {
    filters = append(filters, store.Filter{Field: "shipment.state", Op: "neq", Value: state})
}
```

**Payment States**:
```go
EnumValues: []string{"Pending", "Paid", "Failed", "Refunded"}
```

**Return/Refund States**:
```go
// Return: NotRequested, Requested, Approved, Completed
// Refund: NotInitiated, Pending, Processed, Failed
```

**Customer VIP Tiers**:
```go
EnumValues: []string{"gold", "platinum", "diamond", "silver"}

// Filtering logic hardcoded in parser.go
if !slots.CRMIncludeSilver {
    filters = append(filters, store.Filter{Field: "customer.vip_tier", Op: "neq", Value: "silver"})
}
```

**Files to Change**:
- `internal/semantic/schema_provider.go` (all enum definitions)
- `internal/semantic/parser.go` (filter generation logic)
- `migrations/002_seed.sql` (all demo data uses these enums)
- All tests with hardcoded enum values

---

#### Identifier Patterns & Normalization - **80% Hardcoded, 20% Configurable**
**Impact**: HIGH | **Files Affected**: 6+

**Prefix Patterns Hardcoded**:
```go
// internal/identifier/detector.go:8-16
reOrderNumber    = regexp.MustCompile(`(?i)\bORD-\d{6}\b|\bord-\d{5}\b`)
reTrackingID     = regexp.MustCompile(`(?i)\bTRK-\d{8}\b|\bTB-TRK-\d{6}\b`)
rePaymentRef     = regexp.MustCompile(`(?i)\bPAY-\d{8}\b`)
reCustomerNumber = regexp.MustCompile(`(?i)\bCUST-\d{6}\b|\bcust-\d{5}\b|\bTB-CUST-\d{5}\b`)
```

**Normalization Functions Hardcoded**:
```go
// internal/semantic/parser.go:484-548
func extractOrderNumber(input string) string {
    m := reExtractOrderNumber.FindStringSubmatch(input)
    if len(m) > 1 {
        return "ORD-" + leftPad(m[1], 6)  // ← Hardcoded prefix + format
    }
    return ""
}

func extractTrackingNumber(input string) string {
    return "TRK-" + leftPad(m[1], 8)  // ← Hardcoded prefix + format
}

// Similar for extractPaymentReference, extractCustomerNumber
```

**Identifier Type → Field Mapping Hardcoded**:
```go
// internal/identifier/resolver.go:40-72
switch d.Type {
case TypeOrderNumber:
    groups = append(groups, GroupSpec{
        ResourceType: "order",           // ← Hardcoded
        MatchField:   "order.number",    // ← Hardcoded
        Operator:     op,
        Value:        val,
    })
case TypeTrackingID:
    groups = append(groups, GroupSpec{
        ResourceType: "order",
        MatchField:   "shipment.tracking_id",  // ← Hardcoded
        ...
    })
// ... 5 more cases
}
```

**Partially Configurable** (tenant-specific patterns):
```json
// config/identifier_patterns.json
{
  "tenant-a": {
    "patterns": [
      {
        "name": "tenant_a_order_x_prefix",
        "regex": "(?i)\\bX\\d{4}\\b",
        "type": "order_number"
      }
    ]
  }
}
```

**Files to Change**:
- `internal/identifier/detector.go` (regex patterns)
- `internal/identifier/patterns.go` (default patterns)
- `internal/semantic/parser.go` (all extract* functions)
- `internal/identifier/resolver.go` (type → field mapping switch)
- `config/identifier_patterns.json` (tenant patterns)

---

### 2. BUSINESS LOGIC HARDCODED (Intent & Filter Logic)

#### Intent Classification - **100% Hardcoded**
**Impact**: MEDIUM | **Files Affected**: 2

**Keyword-Based Classification**:
```go
// internal/semantic/parser.go:150-189
func classifyIntent(lower string) string {
    // Returns & Refunds
    if containsAny(lower, "return", "refund", "eligible", "rma") {
        return contracts.IntentReturnsRefunds
    }

    // WISMO (Where Is My Order)
    if containsAny(lower, "tracking", "shipped", "delayed", "where is", "package") {
        return contracts.IntentWISMO
    }

    // CRM Profile
    if containsAny(lower, "customer", "vip", "profile", "segment") {
        return contracts.IntentCRMProfile
    }

    return contracts.IntentDefault
}
```

**Files to Change**:
- `internal/semantic/parser.go` (intent classification)
- `internal/semantic/prompt_builder.go` (intent descriptions for SLM)

---

#### Filter Generation Logic - **100% Hardcoded**
**Impact**: HIGH | **Files Affected**: 1

**State-Based Filters**:
```go
// internal/semantic/parser.go:249-296
func fillWISMOFilters(slots Slots) []store.Filter {
    filters := []store.Filter{}

    if slots.OrderStatusOpen {
        filters = append(filters, store.Filter{
            Field: "order.state",    // ← Hardcoded field
            Op:    "eq",
            Value: "Open",           // ← Hardcoded enum value
        })
    }

    if slots.ShipmentNotShipped {
        // Hardcoded negation of specific shipment states
        deny := []string{"Shipped", "Delivered", "Ready"}
        for _, state := range deny {
            filters = append(filters, store.Filter{
                Field: "shipment.state",
                Op:    "neq",
                Value: state,
            })
        }
    }

    // ... more hardcoded filter logic
}
```

**CRM Filters**:
```go
func fillCRMFilters(slots Slots) []store.Filter {
    if !slots.CRMIncludeSilver {
        return []store.Filter{{
            Field: "customer.vip_tier",  // ← Hardcoded field
            Op:    "neq",
            Value: "silver",             // ← Hardcoded value
        }}
    }
    return []store.Filter{}
}
```

**Files to Change**:
- `internal/semantic/parser.go` (fillWISMOFilters, fillCRMFilters, fillReturnsFilters)

---

#### Few-Shot Examples - **100% Hardcoded**
**Impact**: MEDIUM | **Files Affected**: 1

**All 18 Examples Use Demo Schema**:
```go
// internal/semantic/example_provider.go:40-251
examples := []QueryExample{
    {
        Query:    "ORD-123456",    // ← Demo identifier format
        Intent:   "identifier_lookup",
        Category: contracts.IntentDefault,
        Resource: "order",         // ← Demo resource type
        Filters: []store.Filter{
            {Field: "order.number", Op: "eq", Value: "ORD-123456"},  // ← Demo field
        },
    },
    {
        Query:    "orders not shipped yet",
        Filters: []store.Filter{
            {Field: "shipment.state", Op: "neq", Value: "Shipped"},   // ← Demo enum
            {Field: "shipment.state", Op: "neq", Value: "Delivered"}, // ← Demo enum
            {Field: "shipment.state", Op: "neq", Value: "Ready"},     // ← Demo enum
        },
    },
    // ... 16 more examples all using demo schema
}
```

**Files to Change**:
- `internal/semantic/example_provider.go` (all 18 examples, lines 40-251)

---

### 3. CONFIGURABLE (Can Change Without Code Modification)

#### Identifier Patterns (Tenant-Specific) - **Configurable**
**Impact**: LOW | **Files Affected**: 1

✅ Can add tenant-specific patterns via config:
```json
{
  "tenant-b": {
    "patterns": [
      {
        "name": "custom_order_pattern",
        "regex": "(?i)\\bMYORD-\\d{8}\\b",
        "type": "order_number"
      }
    ]
  }
}
```

**Files to Change**:
- `config/identifier_patterns.json` only

---

#### Query Shape Thresholds - **Configurable**
**Impact**: LOW | **Files Affected**: 1

✅ Can tune typeahead vs. identifier detection:
```json
{
  "typeaheadMinLen": 3,
  "identifierMinLen": 4,
  "shortNoOpLen": 2
}
```

**Files to Change**:
- `config/query_shape_thresholds.json` only

---

### 4. GENERIC CODE (Schema-Agnostic)

✅ **No Changes Needed for Different Schemas**:
- Permission policy engine (`internal/policy`)
- RBAC/ABAC authorization logic (`internal/policy/evaluator.go`)
- HTTP server framework (`internal/http/server.go`)
- Middleware (`internal/http/middleware`)
- Vector retrieval abstraction (`internal/semantic/retriever.go`)
- Pagination/sorting logic (`internal/store/query.go`)
- Logging and tracing (`internal/logging`)

---

## Migration Effort by Component

| Component | Lines of Code | Hardcoded % | Change Effort |
|-----------|--------------|-------------|---------------|
| **Field Definitions** | ~500 | 100% | HIGH - Rewrite all fields |
| **Enum Values** | ~200 | 100% | HIGH - Rewrite all enums |
| **Identifier Patterns** | ~300 | 80% | HIGH - Rewrite patterns + normalization |
| **Resolver Mapping** | ~150 | 100% | HIGH - Rewrite type→field mapping |
| **Intent Classification** | ~100 | 100% | MEDIUM - Rewrite keyword rules |
| **Filter Logic** | ~200 | 100% | HIGH - Rewrite state-based logic |
| **Few-Shot Examples** | ~250 | 100% | MEDIUM - Rewrite all 18 examples |
| **Schema Provider** | 254 | 100% | HIGH - Rewrite entire schema |
| **Database Mappings** | ~200 | 100% | HIGH - Rewrite SQL mappings |
| **Migrations** | ~500 | 100% | HIGH - Rewrite all migrations + seed |
| **Integration Tests** | ~800 | 90% | MEDIUM - Update test data |
| **Tenant Config** | ~50 | 20% | LOW - Add new patterns |

**TOTAL ESTIMATE**: ~3,500 lines of code to modify/rewrite for new schema

---

## Recommendations for Schema-Agnostic Refactoring

If you want to make this system support multiple schemas (e.g., different verticals like healthcare, finance, logistics), consider:

### 1. Extract Schema Definitions to Config Files

**Current**: Hardcoded in Go source files
**Proposed**: JSON/YAML schema definition files

```yaml
# config/schemas/ecommerce.yaml
resources:
  order:
    fields:
      - name: order.number
        type: string
        operators: [eq, like]
        identifierType: order_number
        normalizePattern: "ORD-{6d}"

      - name: order.state
        type: enum
        values: [Open, Confirmed, Complete, Cancelled]
        operators: [eq, neq, in]

  customer:
    fields:
      - name: customer.email
        type: string
        operators: [eq, like]
        identifierType: email

# config/schemas/healthcare.yaml (different vertical)
resources:
  patient:
    fields:
      - name: patient.mrn
        type: string
        identifierType: medical_record_number
        normalizePattern: "MRN-{8d}"
```

**Benefits**:
- Switch schemas without code changes
- Support multi-vertical deployment
- Easy to version and diff

---

### 2. Make Identifier Patterns Fully Configurable

**Current**: Default patterns hardcoded + tenant overrides configurable
**Proposed**: All patterns in config, no defaults in code

```yaml
# config/identifiers/ecommerce.yaml
patterns:
  - name: order_number
    regex: "(?i)\\bORD-\\d{6}\\b"
    normalizeFormat: "ORD-{value:06d}"
    targetField: order.number
    targetResource: order

  - name: tracking_id
    regex: "(?i)\\bTRK-\\d{8}\\b"
    normalizeFormat: "TRK-{value:08d}"
    targetField: shipment.tracking_id
    targetResource: order
```

**Benefits**:
- No code changes for new identifier formats
- Support different prefixes per tenant/vertical
- Version identifier rules separately

---

### 3. Intent Classification via Rule Engine

**Current**: Hardcoded keyword matching in code
**Proposed**: Configurable intent rules

```yaml
# config/intents/ecommerce.yaml
intents:
  - name: wismo
    keywords: [tracking, shipped, delayed, package, where is]
    requiredResource: order
    allowedFields: [order.number, shipment.*, payment.*]

  - name: crm_profile
    keywords: [customer, vip, profile, segment]
    requiredResource: customer
    allowedFields: [customer.*]

# config/intents/healthcare.yaml (different vertical)
intents:
  - name: patient_lookup
    keywords: [patient, mrn, appointment, diagnosis]
    requiredResource: patient
    allowedFields: [patient.mrn, patient.name, appointment.*]
```

**Benefits**:
- Add new intents without code deployment
- Tune classification per vertical
- A/B test intent rules

---

### 4. Filter Logic via Rule Templates

**Current**: Hardcoded filter generation functions
**Proposed**: Template-based filter rules

```yaml
# config/filter_rules/wismo.yaml
slots:
  - slot: OrderStatusOpen
    condition: detected
    filters:
      - field: ${schema.order.state_field}
        op: eq
        value: ${schema.order.open_state_value}

  - slot: ShipmentNotShipped
    condition: detected
    filters:
      - field: ${schema.shipment.state_field}
        op: neq
        value: ${schema.shipment.shipped_state_value}
      - field: ${schema.shipment.state_field}
        op: neq
        value: ${schema.shipment.delivered_state_value}
```

**Benefits**:
- Decouple filter logic from code
- Schema-agnostic slot → filter mapping
- Easier to test and validate rules

---

### 5. Few-Shot Examples from External Store

**Current**: Hardcoded 18 examples in Go code
**Proposed**: Load examples from database or config files

```yaml
# config/examples/wismo.yaml
- query: "ORD-123456"
  intent: identifier_lookup
  filters:
    - field: order.number
      op: eq
      value: "ORD-123456"

- query: "orders not shipped yet"
  intent: search_order
  filters:
    - field: shipment.state
      op: neq
      value: Shipped
```

**Benefits**:
- Update examples without code changes
- A/B test different example sets
- Per-tenant example customization
- Load examples for different verticals

---

### 6. Database Schema Abstraction Layer

**Current**: Direct SQLite field name mapping
**Proposed**: ORM or abstract schema layer

```go
// Proposed abstraction
type SchemaMapper interface {
    GetNativeField(logicalField string) (string, error)
    GetLogicalField(nativeField string) (string, error)
    GetTableName(resourceType string) (string, error)
}

// Load mapping from config
type ConfigBasedMapper struct {
    schema SchemaConfig  // Loaded from YAML/JSON
}
```

**Benefits**:
- Support multiple database schemas
- Swap databases without changing business logic
- Test with in-memory schemas

---

## Conclusion

### Current State
- ⚠️ **70-80% of code is demo-schema-dependent**
- ⚠️ **~3,500 lines require modification for new schema**
- ⚠️ **Minimal abstraction between business logic and schema**
- ⚠️ **Hard to support multiple verticals or multi-tenant schemas**

### Improvement Path
To make the system schema-agnostic, prioritize:

1. **High ROI** (Do first):
   - ✅ Extract field definitions to config files
   - ✅ Make identifier patterns fully configurable
   - ✅ Move few-shot examples to external config

2. **Medium ROI** (Do second):
   - ✅ Intent classification rule engine
   - ✅ Filter template system
   - ✅ Schema provider factory pattern

3. **Low ROI** (Do later):
   - ✅ Full ORM abstraction
   - ✅ Dynamic database schema generation
   - ✅ Multi-schema test framework

### Estimated Refactoring Effort
- **Phase 1** (Config extraction): 2-3 weeks
- **Phase 2** (Rule engines): 2-3 weeks
- **Phase 3** (Full abstraction): 3-4 weeks
- **Total**: 7-10 weeks for complete schema-agnostic rewrite

---

## Quick Reference: What Needs Changing

### For E-Commerce → Different E-Commerce Schema
**Effort**: MEDIUM (1-2 weeks)
- Update field names in contracts/fields.go
- Update enum values in schema_provider.go
- Update identifier patterns in detector.go
- Update few-shot examples
- Rewrite migrations + seed data

### For E-Commerce → Different Vertical (Healthcare, Finance, etc.)
**Effort**: HIGH (4-6 weeks)
- All of the above, plus:
- Rewrite intent classification logic
- Rewrite filter generation logic
- Rewrite resource type switches
- Redesign identifier normalization
- Create new contract allowlists
- Rewrite all integration tests

### For Multi-Vertical Support (Generic Platform)
**Effort**: VERY HIGH (2-3 months)
- Full refactoring to config-driven architecture
- Implement all 6 recommendations above
- Design schema registry system
- Build admin UI for schema management
- Comprehensive testing framework
