# Schema Mapping Template

Fill out this template with YOUR actual schema details before starting migration.

---

## Order Fields

| Demo Field | Your Field | Type | Notes |
|------------|-----------|------|-------|
| order.number | _______________ | string | Your order identifier field |
| order.state | _______________ | enum | Your order status field |
| order.created_at | _______________ | timestamp | Order creation date |
| order.completed_at | _______________ | timestamp | Order completion date (nullable) |
| order.customer_id | _______________ | string | Customer reference |
| order.customer_email | _______________ | string | Customer email (denormalized) |
| order.total_cent_amount | _______________ | int | Total amount in cents |
| order.currency_code | _______________ | string | Currency (USD, EUR, etc.) |
| shipment.state | _______________ | enum | Shipping/delivery status |
| shipment.tracking_id | _______________ | string | Tracking number |
| payment.state | _______________ | enum | Payment status |
| payment.reference | _______________ | string | Payment transaction ID |
| return.eligible | _______________ | boolean | Can be returned? |
| return.status | _______________ | enum | Return status (if applicable) |
| refund.status | _______________ | enum | Refund status (if applicable) |

## Customer Fields

| Demo Field | Your Field | Type | Notes |
|------------|-----------|------|-------|
| customer.number | _______________ | string | Customer identifier |
| customer.email | _______________ | string | Email address |
| customer.first_name | _______________ | string | First name |
| customer.last_name | _______________ | string | Last name |
| customer.created_at | _______________ | timestamp | Registration date |
| customer.vip_tier | _______________ | enum | Loyalty/membership level |
| customer.group | _______________ | string | Customer segment (optional) |
| customer.is_email_verified | _______________ | boolean | Email verified? |

---

## Enum Value Mappings

### Order Status (order.state)

Demo Values: `Open`, `Confirmed`, `Complete`, `Cancelled`

Your Values:
- `Open` → _______________
- `Confirmed` → _______________
- `Complete` → _______________
- `Cancelled` → _______________
- Additional values in your system: _______________

### Shipping Status (shipment.state)

Demo Values: `Pending`, `Shipped`, `Delivered`, `Delayed`, `Ready`

Your Values:
- `Pending` → _______________
- `Shipped` → _______________
- `Delivered` → _______________
- `Delayed` → _______________
- `Ready` → _______________
- Additional values in your system: _______________

### Payment Status (payment.state)

Demo Values: `Pending`, `Paid`, `Failed`, `Refunded`

Your Values:
- `Pending` → _______________
- `Paid` → _______________
- `Failed` → _______________
- `Refunded` → _______________
- Additional values in your system: _______________

### Customer VIP Tier (customer.vip_tier)

Demo Values: `gold`, `platinum`, `diamond`, `silver`

Your Values:
- Tier 1 (lowest): _______________
- Tier 2: _______________
- Tier 3: _______________
- Tier 4 (highest): _______________
- Additional tiers: _______________

### Return Status (if applicable)

Demo Values: `NotRequested`, `Requested`, `Approved`, `Completed`

Your Values:
- Not requested: _______________
- Requested: _______________
- Approved: _______________
- Completed: _______________
- Additional statuses: _______________

### Refund Status (if applicable)

Demo Values: `NotInitiated`, `Pending`, `Processed`, `Failed`

Your Values:
- Not initiated: _______________
- Pending: _______________
- Processed: _______________
- Failed: _______________
- Additional statuses: _______________

---

## Identifier Formats

### Order Identifier

Demo Format: `ORD-001234` (prefix `ORD-` + 6-digit zero-padded number)

Your Format:
- Pattern: _______________
- Example: _______________
- Detection Regex: _______________
- Normalization Rule: _______________

**Examples from your system**:
1. _______________
2. _______________
3. _______________

### Tracking ID

Demo Format: `TRK-12345678` (prefix `TRK-` + 8-digit number)

Your Format:
- Pattern: _______________
- Example: _______________
- Detection Regex: _______________
- Normalization Rule: _______________

**Examples from your system**:
1. _______________
2. _______________
3. _______________

### Customer ID

Demo Format: `CUST-001234` (prefix `CUST-` + 6-digit zero-padded number)

Your Format:
- Pattern: _______________
- Example: _______________
- Detection Regex: _______________
- Normalization Rule: _______________

**Examples from your system**:
1. _______________
2. _______________
3. _______________

### Payment Reference

Demo Format: `PAY-12345678` (prefix `PAY-` + 8-digit number)

Your Format:
- Pattern: _______________
- Example: _______________
- Detection Regex: _______________
- Normalization Rule: _______________

**Examples from your system**:
1. _______________
2. _______________
3. _______________

---

## Database Details

### Table Names

Demo Tables: `orders_docs`, `customers_docs`

Your Tables:
- Orders table: _______________
- Customers table: _______________

### Primary Keys

- Orders PK: _______________
- Customers PK: _______________

### Foreign Keys

- Order → Customer FK: _______________
- Other relationships: _______________

---

## Sample Data (3-5 real examples)

### Sample Orders

```json
// Order 1
{
  "id": "_______________",
  "order_number": "_______________",
  "status": "_______________",
  "customer_id": "_______________",
  "total_amount": _______________,
  "created_at": "_______________",
  "shipping_status": "_______________",
  "tracking_number": "_______________",
  "payment_status": "_______________",
  "payment_id": "_______________"
}

// Order 2
{
  "id": "_______________",
  "order_number": "_______________",
  "status": "_______________",
  "customer_id": "_______________",
  "total_amount": _______________,
  "created_at": "_______________",
  "shipping_status": "_______________",
  "tracking_number": "_______________",
  "payment_status": "_______________",
  "payment_id": "_______________"
}
```

### Sample Customers

```json
// Customer 1
{
  "id": "_______________",
  "customer_number": "_______________",
  "email": "_______________",
  "first_name": "_______________",
  "last_name": "_______________",
  "membership_level": "_______________",
  "created_at": "_______________"
}

// Customer 2
{
  "id": "_______________",
  "customer_number": "_______________",
  "email": "_______________",
  "first_name": "_______________",
  "last_name": "_______________",
  "membership_level": "_______________",
  "created_at": "_______________"
}
```

---

## Migration Checklist

Before starting migration, ensure you have:

- [ ] Filled out all field mappings above
- [ ] Documented all enum value mappings
- [ ] Identified all identifier patterns with examples
- [ ] Collected sample data for testing
- [ ] Created backup of current code: `./scripts/migrate_schema.sh backup`
- [ ] Created backup of database (if exists)
- [ ] Reviewed migration guide: `docs/MIGRATION_GUIDE_SAME_VERTICAL.md`
- [ ] Set aside 1-2 weeks for migration work
- [ ] Planned rollback strategy

---

## Notes & Special Cases

Document any special considerations for your schema:

1. **Nullable Fields**: Which fields can be NULL in your schema?
   - _______________

2. **Denormalized Data**: Any denormalized fields (e.g., customer email on order)?
   - _______________

3. **Calculated Fields**: Any generated/computed columns?
   - _______________

4. **Multi-Tenant Considerations**: How is tenant isolation handled?
   - _______________

5. **Date/Time Formats**: Timezone handling, format specifics
   - _______________

6. **Currency/Amounts**: How are monetary amounts stored?
   - _______________

7. **Other Notes**:
   - _______________
   - _______________
   - _______________

---

## Validation Queries

Provide SQL queries to validate data in your system:

```sql
-- Count total orders
SELECT COUNT(*) FROM _______________ ;

-- Count by order status
SELECT status, COUNT(*)
FROM _______________
GROUP BY status;

-- Count by shipping status
SELECT shipping_status, COUNT(*)
FROM _______________
GROUP BY shipping_status;

-- Count customers by tier
SELECT membership_level, COUNT(*)
FROM _______________
GROUP BY membership_level;

-- Sample recent orders
SELECT * FROM _______________
ORDER BY created_at DESC
LIMIT 10;
```

---

**Once this template is complete, you're ready to start the migration!**

Refer to `docs/MIGRATION_GUIDE_SAME_VERTICAL.md` for step-by-step instructions.
