# Migration Quick Start Guide

## For: Same Vertical (E-Commerce), Different Field Names

If you want to adapt this demo to **your existing e-commerce database schema**, follow this guide.

---

## 📋 What You'll Need

- Your database schema (column names, types)
- Your enum values (order status, shipping status, etc.)
- Your identifier formats (order numbers, tracking IDs, etc.)
- Sample data from your system (5-10 records)
- 1-2 weeks of development time

---

## 🚀 Quick Start (5 Steps)

### Step 1: Fill Out Schema Mapping Template (30 minutes)

Open and complete: `docs/schema_mapping_template.md`

This template captures:
- ✅ Your field names vs. demo field names
- ✅ Your enum values vs. demo enum values
- ✅ Your identifier formats vs. demo formats
- ✅ Sample data for validation

**Example**:
```markdown
| Demo Field | Your Field | Type |
|------------|-----------|------|
| order.number | order_id | string |
| order.state | status | enum |
| shipment.state | shipping_status | enum |
```

---

### Step 2: Create Backup (5 minutes)

```bash
# Create backup of current code and database
./scripts/migrate_schema.sh backup

# Output: backups/schema_migration_20260314_103045/
```

This creates a snapshot you can restore if migration fails.

---

### Step 3: Analyze Current Schema (5 minutes)

```bash
# See what needs changing
./scripts/migrate_schema.sh analyze

# Output shows:
# - Current field names used
# - Current enum values
# - Current identifier patterns
```

This helps you understand the scope of changes.

---

### Step 4: Perform Field Migrations (1-2 days)

Use the migration script to replace field names:

```bash
# Preview changes before applying
./scripts/migrate_schema.sh preview orderNumber order_id

# Apply the change
./scripts/migrate_schema.sh replace orderNumber order_id

# Repeat for each field
./scripts/migrate_schema.sh replace orderState status
./scripts/migrate_schema.sh replace shipmentState shipping_status
./scripts/migrate_schema.sh replace paymentReference payment_id
# ... etc.
```

**Full list of files to manually update**:
1. ✏️ `internal/contracts/fields.go` - Field definitions
2. ✏️ `internal/store/sqlite/adapter.go` - Database mappings
3. ✏️ `internal/semantic/schema_provider.go` - Enum values
4. ✏️ `internal/semantic/parser.go` - Filter logic
5. ✏️ `internal/identifier/detector.go` - Identifier patterns
6. ✏️ `internal/semantic/example_provider.go` - Few-shot examples
7. ✏️ `migrations/001_init.sql` - Database schema
8. ✏️ `migrations/002_seed.sql` - Seed data

**See**: `docs/MIGRATION_GUIDE_SAME_VERTICAL.md` for detailed instructions on each file.

---

### Step 5: Test & Validate (2-3 days)

```bash
# Run all tests
go test ./... -v

# Start API
go run ./cmd/api

# Test with your identifiers
curl -X POST http://localhost:8080/api/query/interpret \
  -H 'Content-Type: application/json' \
  -d '{"message":"YOUR_ORDER_ID_HERE"}' | jq .

# Test semantic queries
curl -X POST http://localhost:8080/api/query/interpret \
  -H 'Content-Type: application/json' \
  -d '{"message":"orders not shipped yet"}' | jq .
```

**Validation checklist**:
- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] API starts without errors
- [ ] Order identifier lookup works
- [ ] Tracking ID lookup works
- [ ] Semantic queries use correct enum values
- [ ] Database queries return results

---

## 📊 Migration Effort Estimate

| Phase | Tasks | Estimated Time |
|-------|-------|----------------|
| **Preparation** | Schema mapping template | 0.5 days |
| **Code Changes** | Update 8 files (~2,000 lines) | 3-4 days |
| **Database Migration** | Update migrations + seed data | 1-2 days |
| **Testing** | Update tests + validation | 2-3 days |
| **Buffer** | Debugging + refinement | 1-2 days |
| **TOTAL** | | **8-12 days** |

---

## 📚 Documentation Reference

| Document | Purpose |
|----------|---------|
| `MIGRATION_GUIDE_SAME_VERTICAL.md` | **Detailed step-by-step guide** (read this!) |
| `schema_mapping_template.md` | Template to fill out with YOUR schema |
| `SCHEMA_DEPENDENCY_ANALYSIS.md` | Full analysis of schema dependencies |
| `../scripts/migrate_schema.sh` | Automated migration helper tool |

---

## 🛠️ Migration Helper Script Usage

The `migrate_schema.sh` script automates find/replace operations:

### Find Occurrences
```bash
./scripts/migrate_schema.sh find orderNumber
# Shows all files where "orderNumber" appears
```

### Preview Changes
```bash
./scripts/migrate_schema.sh preview orderNumber order_id
# Shows what would change without applying
```

### Apply Changes
```bash
./scripts/migrate_schema.sh replace orderNumber order_id
# Replaces across all Go and SQL files
# Prompts for confirmation before applying
```

### Analyze Schema
```bash
./scripts/migrate_schema.sh analyze
# Shows current field names, enums, patterns
```

### Create Backup
```bash
./scripts/migrate_schema.sh backup
# Creates timestamped backup in backups/
```

---

## ⚠️ Common Pitfalls

### 1. Inconsistent Updates
**Problem**: Updated field in one file but forgot others
**Solution**: Use migration script for global find/replace

### 2. Enum Value Mismatches
**Problem**: Code uses "Open" but DB has "Pending"
**Solution**: Update ALL enum references (schema_provider.go + parser.go + examples)

### 3. Identifier Regex Too Strict
**Problem**: Regex doesn't match variations
**Solution**: Test regex with real data first

### 4. Test Data Stale
**Problem**: Code works but tests fail
**Solution**: Update test data to match new schema

### 5. Case Sensitivity
**Problem**: Database uses snake_case, code uses camelCase inconsistently
**Solution**: Standardize on one convention

---

## 🔄 Rollback Plan

If migration fails:

```bash
# Option 1: Git reset (if not committed)
git reset --hard HEAD

# Option 2: Restore from backup
cp -r backups/schema_migration_TIMESTAMP/* ./

# Option 3: Restore specific files
cp backups/schema_migration_TIMESTAMP/internal/contracts/fields.go internal/contracts/

# Rebuild
go clean -cache
go build ./cmd/api
```

---

## ✅ Success Criteria

Migration is complete when:

- ✅ All tests pass: `go test ./... -v`
- ✅ API starts: `go run ./cmd/api`
- ✅ Order lookup works with your ID format
- ✅ Tracking lookup works with your ID format
- ✅ Customer lookup works with your ID format
- ✅ Semantic queries use your enum values
- ✅ Database queries return expected results
- ✅ RBAC policies still enforce correctly
- ✅ No hardcoded demo values remain

---

## 📞 Example Migration Session

Here's what a typical migration looks like:

```bash
# Day 1: Preparation
$ vim docs/schema_mapping_template.md  # Fill out your schema
$ ./scripts/migrate_schema.sh backup  # Create backup
$ ./scripts/migrate_schema.sh analyze # See current state

# Day 2-3: Field Name Changes
$ ./scripts/migrate_schema.sh replace orderNumber order_id
$ ./scripts/migrate_schema.sh replace orderState status
$ ./scripts/migrate_schema.sh replace shipmentState shipping_status
$ ./scripts/migrate_schema.sh replace paymentReference payment_id
$ ./scripts/migrate_schema.sh replace customerNumber customer_id
# ... continue for all fields

# Day 4: Enum Values (manual)
$ vim internal/semantic/schema_provider.go  # Update EnumValues arrays
$ vim internal/semantic/parser.go           # Update filter logic

# Day 5: Identifier Patterns (manual)
$ vim internal/identifier/detector.go       # Update regex patterns
$ vim internal/semantic/parser.go           # Update extract* functions

# Day 6: Examples (manual)
$ vim internal/semantic/example_provider.go # Update all 18 examples

# Day 7: Database (manual)
$ vim migrations/001_init.sql               # Update schema
$ vim migrations/002_seed.sql               # Update seed data

# Day 8-10: Testing
$ go test ./... -v                          # Fix test failures
$ go run ./cmd/api                          # Start API
$ curl ... # Test with real data
```

---

## 🎯 Next Steps

1. **Read this**: `docs/MIGRATION_GUIDE_SAME_VERTICAL.md` (comprehensive guide)
2. **Fill out**: `docs/schema_mapping_template.md` (your schema details)
3. **Create backup**: `./scripts/migrate_schema.sh backup`
4. **Start migrating**: Follow the detailed guide step-by-step
5. **Test thoroughly**: Don't skip validation!

---

## 💡 Tips for Success

- ✅ **Go slow**: Migrate one field at a time, test frequently
- ✅ **Use git**: Commit after each successful phase
- ✅ **Test early**: Run tests after every change
- ✅ **Document**: Keep notes on decisions made
- ✅ **Validate**: Use real production data samples for testing
- ✅ **Ask questions**: Review code diffs carefully

---

## 📈 After Migration

Once migration is complete:

1. **Import real data**: Load your production data for testing
2. **Tune patterns**: Adjust identifier regex based on real traffic
3. **Add examples**: Create few-shot examples from real queries
4. **Monitor accuracy**: Track SLM accuracy with your terminology
5. **Iterate**: Refine based on user feedback

---

**Ready to start?**

👉 Begin with: `docs/MIGRATION_GUIDE_SAME_VERTICAL.md`

Good luck! 🚀
