# Migration Guide: Same Vertical, Different Field Names

## Overview

This guide helps you adapt the permission-aware search system to **your existing e-commerce schema** while keeping the same business logic (orders, customers, shipments, payments).

**Use Case**: You have an e-commerce database with different field names than the demo schema.

**Example**:
- Demo uses: `orderNumber`, `shipmentState`, `paymentReference`
- Your DB uses: `order_id`, `shipping_status`, `transaction_ref`

**Estimated Effort**: 1-2 weeks (depending on schema complexity)

---

## Prerequisites

Before starting, gather your schema details:

1. **Database Schema** - Column names, types, relationships
2. **Enum Values** - Valid values for status fields (order status, shipping status, payment status)
3. **Identifier Formats** - How you identify orders, customers, tracking IDs
4. **Sample Data** - Representative data for testing

---

## Step-by-Step Migration Checklist

### Phase 1: Schema Mapping (Day 1-2)

#### ✅ Step 1.1: Create Schema Mapping Document

Create a mapping table between demo schema and your schema:

```markdown
# schema_mapping.md

## Order Fields

| Demo Field | Your Field | Type | Notes |
|------------|-----------|------|-------|
| order.number | order.order_id | string | Format: ORD-XXXXXX → Your format |
| order.state | order.status | enum | Open/Confirmed/Complete → Pending/Processing/Shipped/Delivered |
| order.created_at | order.order_date | timestamp | Same |
| order.total_cent_amount | order.total_amount | int | Cents → Your currency unit |
| shipment.state | order.shipping_status | enum | Pending/Shipped/Delivered → Your statuses |
| shipment.tracking_id | order.tracking_number | string | TRK-XXXXXXXX → Your format |
| payment.state | order.payment_status | enum | Pending/Paid/Failed → Your statuses |
| payment.reference | order.payment_id | string | PAY-XXXXXXXX → Your format |
| return.eligible | order.returnable | boolean | true/false |
| return.status | order.return_status | enum | Your return statuses |

## Customer Fields

| Demo Field | Your Field | Type | Notes |
|------------|-----------|------|-------|
| customer.number | customer.customer_id | string | CUST-XXXXXX → Your format |
| customer.email | customer.email_address | string | Same concept |
| customer.vip_tier | customer.membership_level | enum | gold/platinum/diamond → bronze/silver/gold/platinum |
| customer.first_name | customer.fname | string | Same |
| customer.last_name | customer.lname | string | Same |
| customer.created_at | customer.registration_date | timestamp | Same |
```

**Action**: Fill in YOUR actual field names in the "Your Field" column

---

#### ✅ Step 1.2: Map Enum Values

Document your enum value mappings:

```markdown
# enum_mapping.md

## Order Status
Demo: Open, Confirmed, Complete, Cancelled
Your: Pending, Processing, Shipped, Delivered, Cancelled

Mapping:
- Open → Pending
- Confirmed → Processing
- Complete → Delivered
- Cancelled → Cancelled

## Shipping Status
Demo: Pending, Shipped, Delivered, Delayed, Ready
Your: NotShipped, InTransit, OutForDelivery, Delivered, Delayed

Mapping:
- Pending → NotShipped
- Shipped → InTransit
- Ready → OutForDelivery
- Delivered → Delivered
- Delayed → Delayed

## Payment Status
Demo: Pending, Paid, Failed, Refunded
Your: Unpaid, Completed, Failed, Refunded

Mapping:
- Pending → Unpaid
- Paid → Completed
- Failed → Failed
- Refunded → Refunded
```

**Action**: Map ALL enum fields in your schema

---

#### ✅ Step 1.3: Map Identifier Formats

Document how identifiers are formatted in your system:

```markdown
# identifier_mapping.md

## Order Identifiers
Demo Format: ORD-001234 (prefix + 6-digit zero-padded)
Your Format: ORDER-2024-12345 (prefix + year + sequential)

Detection Regex: (?i)\bORDER-\d{4}-\d{5}\b
Normalization: Extract number, format as ORDER-{year}-{number:05d}

## Tracking IDs
Demo Format: TRK-12345678 (prefix + 8-digit)
Your Format: SHIP1234567890 (prefix + 10-digit)

Detection Regex: (?i)\bSHIP\d{10}\b
Normalization: Extract digits, format as SHIP{number:010d}

## Customer IDs
Demo Format: CUST-001234 (prefix + 6-digit)
Your Format: C123456 (single letter + 6-digit)

Detection Regex: (?i)\bC\d{6}\b
Normalization: Extract number, format as C{number:06d}

## Payment References
Demo Format: PAY-12345678 (prefix + 8-digit)
Your Format: TXN-20240314-ABC123 (prefix + date + alphanumeric)

Detection Regex: (?i)\bTXN-\d{8}-[A-Z0-9]+\b
Normalization: Keep as-is (no padding needed)
```

**Action**: Document ALL identifier formats you need to support

---

### Phase 2: Code Changes (Day 3-5)

#### ✅ Step 2.1: Update Field Definitions

**File**: `internal/contracts/fields.go`

**Before**:
```go
var OrderFieldsV2 = map[string]FieldDef{
    "order.number": {
        LogicalName:  "order.number",
        NativeColumn: "orderNumber",
        Type:         "string",
    },
    "order.state": {
        LogicalName:  "order.state",
        NativeColumn: "orderState",
        Type:         "enum",
    },
    // ... more fields
}
```

**After** (using your schema):
```go
var OrderFieldsV2 = map[string]FieldDef{
    "order.number": {
        LogicalName:  "order.number",
        NativeColumn: "order_id",              // ← Changed to your column name
        Type:         "string",
    },
    "order.state": {
        LogicalName:  "order.state",
        NativeColumn: "status",                // ← Changed to your column name
        Type:         "enum",
    },
    "shipment.state": {
        LogicalName:  "shipment.state",
        NativeColumn: "shipping_status",       // ← Changed to your column name
        Type:         "enum",
    },
    "payment.reference": {
        LogicalName:  "payment.reference",
        NativeColumn: "payment_id",            // ← Changed to your column name
        Type:         "string",
    },
    // ... update ALL fields
}

var CustomerFieldsV2 = map[string]FieldDef{
    "customer.email": {
        LogicalName:  "customer.email",
        NativeColumn: "email_address",         // ← Changed
        Type:         "string",
    },
    "customer.vip_tier": {
        LogicalName:  "customer.vip_tier",
        NativeColumn: "membership_level",      // ← Changed
        Type:         "enum",
    },
    // ... update ALL fields
}
```

**Important**: Keep the `LogicalName` the same (this is the API contract). Only change `NativeColumn` to match your database.

**Files to update**:
- ✏️ `internal/contracts/fields.go` - Update all NativeColumn values

---

#### ✅ Step 2.2: Update Database Mappings

**File**: `internal/store/sqlite/adapter.go`

**Before**:
```go
func nativeOrderField(logicalField string) (string, bool) {
    switch logicalField {
    case "order.number":
        return "orderNumber", true
    case "order.state":
        return "orderState", true
    case "shipment.state":
        return "shipmentState", true
    case "payment.reference":
        return "paymentReference", true
    // ... more cases
    }
    return "", false
}
```

**After** (using your schema):
```go
func nativeOrderField(logicalField string) (string, bool) {
    switch logicalField {
    case "order.number":
        return "order_id", true              // ← Your column name
    case "order.state":
        return "status", true                // ← Your column name
    case "shipment.state":
        return "shipping_status", true       // ← Your column name
    case "payment.reference":
        return "payment_id", true            // ← Your column name
    // ... update ALL cases
    }
    return "", false
}

func nativeCustomerField(logicalField string) (string, bool) {
    switch logicalField {
    case "customer.email":
        return "email_address", true         // ← Your column name
    case "customer.vip_tier":
        return "membership_level", true      // ← Your column name
    // ... update ALL cases
    }
    return "", false
}
```

**Files to update**:
- ✏️ `internal/store/sqlite/adapter.go` - Update nativeOrderField() and nativeCustomerField()

---

#### ✅ Step 2.3: Update Enum Values in Schema Provider

**File**: `internal/semantic/schema_provider.go`

**Before**:
```go
{
    Name:         "order.state",
    Type:         "enum",
    Description:  "Current state of the order",
    EnumValues:   []string{"Open", "Confirmed", "Complete", "Cancelled"},
    Operators:    []string{"eq", "neq", "in"},
    IntentScopes: []string{IntentWISMO, IntentDefault},
    Example:      "Open",
},
```

**After** (using your enum values):
```go
{
    Name:         "order.state",
    Type:         "enum",
    Description:  "Current state of the order",
    EnumValues:   []string{"Pending", "Processing", "Shipped", "Delivered", "Cancelled"},  // ← Your values
    Operators:    []string{"eq", "neq", "in"},
    IntentScopes: []string{IntentWISMO, IntentDefault},
    Example:      "Pending",  // ← Your default value
},
{
    Name:         "shipment.state",
    Type:         "enum",
    Description:  "Current shipping status",
    EnumValues:   []string{"NotShipped", "InTransit", "OutForDelivery", "Delivered", "Delayed"},  // ← Your values
    Operators:    []string{"eq", "neq", "in"},
    IntentScopes: []string{IntentWISMO, IntentDefault},
    Example:      "InTransit",
},
{
    Name:         "customer.vip_tier",
    Type:         "enum",
    Description:  "Customer membership level",
    EnumValues:   []string{"bronze", "silver", "gold", "platinum"},  // ← Your values (changed from gold/platinum/diamond)
    Operators:    []string{"eq", "neq", "in"},
    IntentScopes: []string{IntentCRMProfile, IntentDefault},
    Example:      "gold",
},
```

**Files to update**:
- ✏️ `internal/semantic/schema_provider.go` - Update ALL EnumValues arrays

---

#### ✅ Step 2.4: Update Filter Logic

**File**: `internal/semantic/parser.go`

**Before**:
```go
func fillWISMOFilters(slots Slots) []store.Filter {
    filters := []store.Filter{}

    if slots.OrderStatusOpen {
        filters = append(filters, store.Filter{
            Field: "order.state",
            Op:    "eq",
            Value: "Open",  // ← Demo enum value
        })
    }

    if slots.ShipmentNotShipped {
        deny := []string{"Shipped", "Delivered", "Ready"}  // ← Demo enum values
        for _, state := range deny {
            filters = append(filters, store.Filter{
                Field: "shipment.state",
                Op:    "neq",
                Value: state,
            })
        }
    }

    return filters
}
```

**After** (using your enum values):
```go
func fillWISMOFilters(slots Slots) []store.Filter {
    filters := []store.Filter{}

    if slots.OrderStatusOpen {
        filters = append(filters, store.Filter{
            Field: "order.state",
            Op:    "eq",
            Value: "Pending",  // ← Your enum value for "open/pending" orders
        })
    }

    if slots.ShipmentNotShipped {
        // Deny orders that have already shipped
        deny := []string{"InTransit", "OutForDelivery", "Delivered"}  // ← Your enum values
        for _, state := range deny {
            filters = append(filters, store.Filter{
                Field: "shipment.state",
                Op:    "neq",
                Value: state,
            })
        }
    }

    return filters
}

func fillCRMFilters(slots Slots) []store.Filter {
    if !slots.CRMIncludeSilver {
        return []store.Filter{{
            Field: "customer.vip_tier",
            Op:    "neq",
            Value: "bronze",  // ← Changed from "silver" to your lowest tier
        }}
    }
    return []store.Filter{}
}
```

**Files to update**:
- ✏️ `internal/semantic/parser.go` - Update fillWISMOFilters(), fillCRMFilters(), fillReturnsFilters()

---

#### ✅ Step 2.5: Update Identifier Patterns

**File**: `internal/identifier/detector.go`

**Before**:
```go
var (
    reOrderNumber    = regexp.MustCompile(`(?i)\bORD-\d{6}\b|\bord-\d{5}\b`)
    reTrackingID     = regexp.MustCompile(`(?i)\bTRK-\d{8}\b|\bTB-TRK-\d{6}\b`)
    rePaymentRef     = regexp.MustCompile(`(?i)\bPAY-\d{8}\b`)
    reCustomerNumber = regexp.MustCompile(`(?i)\bCUST-\d{6}\b|\bcust-\d{5}\b`)
)
```

**After** (using your identifier formats):
```go
var (
    reOrderNumber    = regexp.MustCompile(`(?i)\bORDER-\d{4}-\d{5}\b`)  // ← Your format: ORDER-2024-12345
    reTrackingID     = regexp.MustCompile(`(?i)\bSHIP\d{10}\b`)         // ← Your format: SHIP1234567890
    rePaymentRef     = regexp.MustCompile(`(?i)\bTXN-\d{8}-[A-Z0-9]+\b`) // ← Your format: TXN-20240314-ABC123
    reCustomerNumber = regexp.MustCompile(`(?i)\bC\d{6}\b`)             // ← Your format: C123456
)
```

**Files to update**:
- ✏️ `internal/identifier/detector.go` - Update regex patterns

---

#### ✅ Step 2.6: Update Identifier Normalization

**File**: `internal/semantic/parser.go`

**Before**:
```go
var (
    reExtractOrderNumber    = regexp.MustCompile(`(?i)(?:ORD-?)?(\d{4,6})`)
    reExtractTrackingNumber = regexp.MustCompile(`(?i)(?:TRK-?)?(\d{6,8})`)
    reExtractPaymentRef     = regexp.MustCompile(`(?i)(?:PAY-?)?(\d{6,8})`)
    reExtractCustomerNumber = regexp.MustCompile(`(?i)(?:CUST-?)?(\d{4,6})`)
)

func extractOrderNumber(input string) string {
    m := reExtractOrderNumber.FindStringSubmatch(input)
    if len(m) > 1 {
        return "ORD-" + leftPad(m[1], 6)  // ← Demo normalization
    }
    return ""
}
```

**After** (using your formats):
```go
var (
    reExtractOrderNumber    = regexp.MustCompile(`(?i)(?:ORDER-)?(\d{4})-?(\d{5})`)
    reExtractTrackingNumber = regexp.MustCompile(`(?i)(?:SHIP)?(\d{10})`)
    reExtractPaymentRef     = regexp.MustCompile(`(?i)(TXN-\d{8}-[A-Z0-9]+)`)
    reExtractCustomerNumber = regexp.MustCompile(`(?i)C?(\d{6})`)
)

func extractOrderNumber(input string) string {
    m := reExtractOrderNumber.FindStringSubmatch(input)
    if len(m) > 2 {
        year := m[1]
        num := m[2]
        return fmt.Sprintf("ORDER-%s-%s", year, leftPad(num, 5))  // ← Your normalization
    }
    return ""
}

func extractTrackingNumber(input string) string {
    m := reExtractTrackingNumber.FindStringSubmatch(input)
    if len(m) > 1 {
        return "SHIP" + leftPad(m[1], 10)  // ← Your normalization
    }
    return ""
}

func extractPaymentReference(input string) string {
    m := reExtractPaymentRef.FindStringSubmatch(input)
    if len(m) > 1 {
        return strings.ToUpper(m[1])  // ← Your normalization (keep as-is, uppercase)
    }
    return ""
}

func extractCustomerNumber(input string) string {
    m := reExtractCustomerNumber.FindStringSubmatch(input)
    if len(m) > 1 {
        return "C" + leftPad(m[1], 6)  // ← Your normalization
    }
    return ""
}
```

**Files to update**:
- ✏️ `internal/semantic/parser.go` - Update all extract* functions
- ✏️ `internal/identifier/detector.go` - Update normalizeByType() function

---

#### ✅ Step 2.7: Update Few-Shot Examples

**File**: `internal/semantic/example_provider.go`

**Before**:
```go
examples := []QueryExample{
    {
        Query:    "ORD-123456",
        Intent:   "identifier_lookup",
        Category: contracts.IntentDefault,
        Resource: "order",
        Filters: []store.Filter{
            {Field: "order.number", Op: "eq", Value: "ORD-123456"},
        },
        Confidence: 0.95,
    },
    {
        Query:    "orders not shipped yet",
        Intent:   "search_order",
        Category: contracts.IntentWISMO,
        Resource: "order",
        Filters: []store.Filter{
            {Field: "shipment.state", Op: "neq", Value: "Shipped"},
            {Field: "shipment.state", Op: "neq", Value: "Delivered"},
            {Field: "shipment.state", Op: "neq", Value: "Ready"},
        },
        Confidence: 0.9,
    },
    // ... 16 more examples
}
```

**After** (using your schema):
```go
examples := []QueryExample{
    {
        Query:    "ORDER-2024-12345",  // ← Your identifier format
        Intent:   "identifier_lookup",
        Category: contracts.IntentDefault,
        Resource: "order",
        Filters: []store.Filter{
            {Field: "order.number", Op: "eq", Value: "ORDER-2024-12345"},
        },
        Confidence: 0.95,
    },
    {
        Query:    "orders not shipped yet",
        Intent:   "search_order",
        Category: contracts.IntentWISMO,
        Resource: "order",
        Filters: []store.Filter{
            {Field: "shipment.state", Op: "neq", Value: "InTransit"},       // ← Your enum
            {Field: "shipment.state", Op: "neq", Value: "OutForDelivery"},  // ← Your enum
            {Field: "shipment.state", Op: "neq", Value: "Delivered"},       // ← Your enum
        },
        Confidence: 0.9,
    },
    {
        Query:    "pending orders",
        Intent:   "search_order",
        Category: contracts.IntentWISMO,
        Resource: "order",
        Filters: []store.Filter{
            {Field: "order.state", Op: "eq", Value: "Pending"},  // ← Your enum (was "Open")
        },
        Sort:       store.Sort{Field: "order.created_at", Dir: "desc"},
        Confidence: 0.9,
    },
    // ... update ALL 18 examples
}
```

**Files to update**:
- ✏️ `internal/semantic/example_provider.go` - Update ALL 18 examples

---

### Phase 3: Database Migration (Day 6-7)

#### ✅ Step 3.1: Create New Migration

**File**: `migrations/001_init.sql` (create a copy as `001_init_yourschema.sql`)

**Before**:
```sql
CREATE TABLE IF NOT EXISTS orders_docs (
    id TEXT PRIMARY KEY,
    tenantId TEXT NOT NULL,
    orderNumber TEXT NOT NULL,
    orderState TEXT NOT NULL,
    shipmentState TEXT NOT NULL,
    paymentState TEXT NOT NULL,
    -- ... more columns
);
```

**After** (using your schema):
```sql
CREATE TABLE IF NOT EXISTS orders_docs (
    id TEXT PRIMARY KEY,
    tenantId TEXT NOT NULL,
    order_id TEXT NOT NULL,           -- ← Your column name
    status TEXT NOT NULL,              -- ← Your column name (order.state)
    shipping_status TEXT NOT NULL,     -- ← Your column name (shipment.state)
    payment_status TEXT NOT NULL,      -- ← Your column name (payment.state)
    payment_id TEXT,                   -- ← Your column name (payment.reference)
    tracking_number TEXT,              -- ← Your column name (shipment.tracking_id)
    -- ... update ALL column names
);

-- Update indexes with new column names
CREATE INDEX IF NOT EXISTS idx_orders_order_id ON orders_docs(order_id);
CREATE INDEX IF NOT EXISTS idx_orders_shipping_status ON orders_docs(shipping_status);
CREATE INDEX IF NOT EXISTS idx_orders_tracking_number ON orders_docs(tracking_number);
```

**Files to update**:
- ✏️ `migrations/001_init.sql` - Update table schema with your column names

---

#### ✅ Step 3.2: Create New Seed Data

**File**: `migrations/002_seed.sql`

**Before**:
```sql
INSERT INTO orders_docs (id, tenantId, orderNumber, orderState, shipmentState, paymentState, ...)
VALUES
    ('ord-a1', 'tenant-a', 'ORD-001004', 'Open', 'Pending', 'Pending', ...),
    ('ord-a2', 'tenant-a', 'ORD-001005', 'Confirmed', 'Shipped', 'Paid', ...);
```

**After** (using your data):
```sql
INSERT INTO orders_docs (id, tenantId, order_id, status, shipping_status, payment_status, ...)
VALUES
    ('ord-a1', 'tenant-a', 'ORDER-2024-10001', 'Pending', 'NotShipped', 'Unpaid', ...),
    ('ord-a2', 'tenant-a', 'ORDER-2024-10002', 'Processing', 'InTransit', 'Completed', ...);
```

**Better Option**: If you have existing data, write a data import script instead:

```bash
# scripts/import_existing_data.sh
#!/bin/bash

# Export from your production DB
pg_dump -h your-db-host -U user -d your_db --table=orders --data-only > /tmp/orders.sql

# Transform data to match new schema
# (Use Python/Go script to map your columns to the expected schema)

# Import into SQLite
sqlite3 data/search.db < /tmp/transformed_orders.sql
```

**Files to create**:
- ✏️ `scripts/import_existing_data.sh` - Data import script
- ✏️ `scripts/transform_schema.py` - Schema transformation script

---

### Phase 4: Testing & Validation (Day 8-10)

#### ✅ Step 4.1: Update Integration Tests

**File**: `cmd/api/identifier_integration_test.go`

**Before**:
```go
func TestIdentifierDetection(t *testing.T) {
    tests := []struct {
        input    string
        expected string
    }{
        {"ORD-001004", "order_number"},
        {"TRK-12345678", "tracking_id"},
        {"CUST-123456", "customer_number"},
    }
    // ...
}
```

**After** (using your formats):
```go
func TestIdentifierDetection(t *testing.T) {
    tests := []struct {
        input    string
        expected string
    }{
        {"ORDER-2024-12345", "order_number"},  // ← Your format
        {"SHIP1234567890", "tracking_id"},     // ← Your format
        {"C123456", "customer_number"},        // ← Your format
    }
    // ...
}
```

**Files to update**:
- ✏️ All test files in `internal/identifier/*_test.go`
- ✏️ All test files in `internal/semantic/*_test.go`
- ✏️ Integration tests in `cmd/api/*_test.go`

---

#### ✅ Step 4.2: Run Test Suite

```bash
# Run all tests
go test ./... -v

# Run specific package tests
go test ./internal/identifier -v
go test ./internal/semantic -v
go test ./internal/store -v

# Run integration tests
go test ./cmd/api -v
```

**Expected Failures**: Tests will fail until ALL schema references are updated.

**Fix Strategy**:
1. Run tests
2. Fix failures one by one
3. Re-run tests
4. Repeat until all pass

---

#### ✅ Step 4.3: Manual API Testing

Start the API and test with your actual identifiers:

```bash
# Start API
go run ./cmd/api

# Test order lookup with your format
curl -X POST http://localhost:8080/api/query/interpret \
  -H 'Content-Type: application/json' \
  -H 'X-User-Id: alice' \
  -H 'X-Tenant-Id: tenant-a' \
  -d '{"message":"ORDER-2024-12345","debug":true}' | jq .

# Test tracking lookup
curl -X POST http://localhost:8080/api/query/interpret \
  -H 'Content-Type: application/json' \
  -H 'X-User-Id: alice' \
  -H 'X-Tenant-Id: tenant-a' \
  -d '{"message":"SHIP1234567890","debug":true}' | jq .

# Test semantic query with your enum values
curl -X POST http://localhost:8080/api/query/interpret \
  -H 'Content-Type: application/json' \
  -H 'X-User-Id: alice' \
  -H 'X-Tenant-Id: tenant-a' \
  -d '{"message":"orders not shipped yet","debug":true}' | jq .
```

**Validate**:
- ✅ Identifier detection works with your formats
- ✅ Enum filters use your values
- ✅ Database queries return results
- ✅ Field names match your schema

---

## Quick Reference: Files to Modify

### Critical Files (Must Update)

| File | What to Change | Lines |
|------|----------------|-------|
| `internal/contracts/fields.go` | NativeColumn values | ~500 |
| `internal/store/sqlite/adapter.go` | nativeOrderField(), nativeCustomerField() | ~200 |
| `internal/semantic/schema_provider.go` | EnumValues arrays | ~250 |
| `internal/semantic/parser.go` | Filter values, extract* functions | ~300 |
| `internal/identifier/detector.go` | Regex patterns | ~50 |
| `internal/semantic/example_provider.go` | All 18 examples | ~250 |
| `migrations/001_init.sql` | Table column names | ~200 |
| `migrations/002_seed.sql` | Seed data values | ~300 |

**Total**: ~2,050 lines to modify

### Test Files (Update After Code Changes)

| File | What to Change |
|------|----------------|
| `internal/identifier/*_test.go` | Test data with your identifiers |
| `internal/semantic/*_test.go` | Test data with your enum values |
| `cmd/api/*_test.go` | Integration test data |

---

## Migration Helper Script

Create a script to help with the migration:

```bash
# scripts/migrate_schema.sh
#!/bin/bash

# This script helps migrate field names across the codebase

OLD_FIELD=$1
NEW_FIELD=$2

if [ -z "$OLD_FIELD" ] || [ -z "$NEW_FIELD" ]; then
    echo "Usage: ./migrate_schema.sh <old_field> <new_field>"
    echo "Example: ./migrate_schema.sh orderNumber order_id"
    exit 1
fi

echo "Replacing '$OLD_FIELD' with '$NEW_FIELD'..."

# Find all occurrences (preview)
echo "Occurrences found:"
grep -r "$OLD_FIELD" internal/ migrations/ --include="*.go" --include="*.sql" | wc -l

# Ask for confirmation
read -p "Proceed with replacement? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    # Replace in Go files
    find internal/ -name "*.go" -exec sed -i '' "s/$OLD_FIELD/$NEW_FIELD/g" {} +

    # Replace in SQL files
    find migrations/ -name "*.sql" -exec sed -i '' "s/$OLD_FIELD/$NEW_FIELD/g" {} +

    echo "✅ Replacement complete!"
    echo "⚠️  Please review changes with: git diff"
else
    echo "❌ Cancelled"
fi
```

**Usage**:
```bash
chmod +x scripts/migrate_schema.sh

# Migrate field names one by one
./scripts/migrate_schema.sh orderNumber order_id
./scripts/migrate_schema.sh orderState status
./scripts/migrate_schema.sh shipmentState shipping_status
./scripts/migrate_schema.sh paymentReference payment_id
```

---

## Validation Checklist

After migration, verify:

- [ ] All tests pass: `go test ./... -v`
- [ ] API starts without errors: `go run ./cmd/api`
- [ ] Order lookup works with your identifier format
- [ ] Tracking lookup works with your identifier format
- [ ] Customer lookup works with your identifier format
- [ ] Semantic queries use your enum values correctly
- [ ] Database queries return results
- [ ] Permission filtering still works
- [ ] RBAC policies still enforce correctly
- [ ] Integration tests pass with new data

---

## Common Pitfalls

### 1. **Inconsistent Field Name Updates**
❌ Problem: Updated field in contracts but forgot adapter
✅ Solution: Use migration script to find-replace globally, then review

### 2. **Case Sensitivity Issues**
❌ Problem: Database uses `order_id` but code has `Order_ID`
✅ Solution: Standardize on snake_case for DB, keep LogicalName as-is

### 3. **Enum Value Typos**
❌ Problem: Filter uses `"Shipped"` but DB has `"InTransit"`
✅ Solution: Create enum constants instead of string literals

### 4. **Identifier Regex Too Strict**
❌ Problem: Regex doesn't match variations of your identifier
✅ Solution: Test regex with real production data samples

### 5. **Missed Test Updates**
❌ Problem: Code works but tests fail with old data
✅ Solution: Update tests last, use them to validate migration

---

## Rollback Plan

If migration fails, rollback steps:

1. **Revert code changes**:
   ```bash
   git reset --hard HEAD
   ```

2. **Restore original database**:
   ```bash
   cp data/search.db.backup data/search.db
   ```

3. **Clear compiled binaries**:
   ```bash
   rm -rf bin/
   go clean -cache
   ```

4. **Restart from checkpoint**:
   - Identify what broke (check git diff)
   - Fix that specific issue
   - Re-run tests
   - Continue migration

---

## Summary

**Total Effort**: 1-2 weeks (8-10 days)

**Complexity**: MEDIUM (straightforward but requires attention to detail)

**Risk**: LOW (no architectural changes, just field/value remapping)

**Success Criteria**:
- ✅ All tests pass
- ✅ API works with your identifier formats
- ✅ Queries use your enum values
- ✅ Database matches your schema
- ✅ No hardcoded demo values remain

**Next Steps After Migration**:
1. Load your actual production data
2. Test with real user queries
3. Tune identifier patterns based on real traffic
4. Monitor SLM accuracy with your specific terminology
5. Add more few-shot examples from real usage

---

## Support Files Created

Along with this guide, you should create:

1. ✅ `schema_mapping.md` - Your field name mappings
2. ✅ `enum_mapping.md` - Your enum value mappings
3. ✅ `identifier_mapping.md` - Your identifier format specs
4. ✅ `scripts/migrate_schema.sh` - Automated migration helper
5. ✅ `scripts/import_existing_data.sh` - Data import from your DB
6. ✅ `data/search.db.backup` - Backup before migration

**All files should be version controlled for rollback safety.**
