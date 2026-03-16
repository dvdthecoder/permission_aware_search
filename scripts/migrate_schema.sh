#!/bin/bash

# Schema Migration Helper Script
# Helps find and replace field names across the codebase

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_info() {
    echo -e "${BLUE}ℹ ${NC}$1"
}

print_success() {
    echo -e "${GREEN}✓ ${NC}$1"
}

print_warning() {
    echo -e "${YELLOW}⚠ ${NC}$1"
}

print_error() {
    echo -e "${RED}✗ ${NC}$1"
}

# Function to show usage
show_usage() {
    cat <<EOF
Schema Migration Helper Script

Usage:
  ./migrate_schema.sh find <pattern>              - Find all occurrences of a pattern
  ./migrate_schema.sh replace <old> <new>         - Replace field name globally
  ./migrate_schema.sh preview <old> <new>         - Preview replacement without applying
  ./migrate_schema.sh analyze                     - Analyze current schema usage
  ./migrate_schema.sh backup                      - Create backup before migration

Examples:
  ./migrate_schema.sh find orderNumber
  ./migrate_schema.sh replace orderNumber order_id
  ./migrate_schema.sh preview orderState status
  ./migrate_schema.sh analyze
  ./migrate_schema.sh backup

EOF
}

# Function to find pattern
find_pattern() {
    local pattern=$1

    print_info "Searching for pattern: '$pattern'"
    echo ""

    # Search in Go files
    echo "Go files:"
    grep -r "$pattern" internal/ cmd/ --include="*.go" --color=always -n | head -20
    local go_count=$(grep -r "$pattern" internal/ cmd/ --include="*.go" | wc -l | tr -d ' ')

    echo ""
    echo "SQL files:"
    grep -r "$pattern" migrations/ --include="*.sql" --color=always -n | head -10
    local sql_count=$(grep -r "$pattern" migrations/ --include="*.sql" | wc -l | tr -d ' ')

    echo ""
    print_info "Total occurrences:"
    echo "  Go files: $go_count"
    echo "  SQL files: $sql_count"
    echo "  TOTAL: $((go_count + sql_count))"
}

# Function to preview replacement
preview_replacement() {
    local old=$1
    local new=$2

    print_info "Preview: Replacing '$old' → '$new'"
    echo ""

    # Preview in Go files
    print_info "Go files (first 10 changes):"
    grep -r "$old" internal/ cmd/ --include="*.go" -n | head -10 | while read -r line; do
        echo "$line" | sed "s/$old/${GREEN}$new${NC}/g"
    done

    echo ""
    # Preview in SQL files
    print_info "SQL files (first 5 changes):"
    grep -r "$old" migrations/ --include="*.sql" -n | head -5 | while read -r line; do
        echo "$line" | sed "s/$old/${GREEN}$new${NC}/g"
    done

    echo ""
    local go_count=$(grep -r "$old" internal/ cmd/ --include="*.go" | wc -l | tr -d ' ')
    local sql_count=$(grep -r "$old" migrations/ --include="*.sql" | wc -l | tr -d ' ')
    local total=$((go_count + sql_count))

    print_warning "This will modify $total occurrences ($go_count in Go, $sql_count in SQL)"
}

# Function to perform replacement
perform_replacement() {
    local old=$1
    local new=$2

    print_info "Replacing '$old' with '$new'..."
    echo ""

    # Count before
    local go_count=$(grep -r "$old" internal/ cmd/ --include="*.go" | wc -l | tr -d ' ')
    local sql_count=$(grep -r "$old" migrations/ --include="*.sql" | wc -l | tr -d ' ')
    local total=$((go_count + sql_count))

    print_warning "Found $total occurrences to replace"
    echo ""
    read -p "Proceed with replacement? (y/n) " -n 1 -r
    echo

    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_error "Replacement cancelled"
        exit 1
    fi

    # Replace in Go files
    print_info "Replacing in Go files..."
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        find internal/ cmd/ -name "*.go" -exec sed -i '' "s/$old/$new/g" {} +
    else
        # Linux
        find internal/ cmd/ -name "*.go" -exec sed -i "s/$old/$new/g" {} +
    fi

    # Replace in SQL files
    print_info "Replacing in SQL files..."
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        find migrations/ -name "*.sql" -exec sed -i '' "s/$old/$new/g" {} +
    else
        # Linux
        find migrations/ -name "*.sql" -exec sed -i "s/$old/$new/g" {} +
    fi

    # Verify
    local go_remaining=$(grep -r "$old" internal/ cmd/ --include="*.go" | wc -l | tr -d ' ')
    local sql_remaining=$(grep -r "$old" migrations/ --include="*.sql" | wc -l | tr -d ' ')

    echo ""
    if [ "$go_remaining" -eq 0 ] && [ "$sql_remaining" -eq 0 ]; then
        print_success "Replacement complete! Replaced $total occurrences"
        print_warning "Please review changes with: git diff"
    else
        print_warning "Some occurrences may remain:"
        echo "  Go files: $go_remaining"
        echo "  SQL files: $sql_remaining"
        print_info "Review with: ./migrate_schema.sh find '$old'"
    fi
}

# Function to analyze schema
analyze_schema() {
    print_info "Analyzing current schema usage..."
    echo ""

    # Field names
    print_info "Top 20 database field names:"
    grep -roh '\b[a-zA-Z_][a-zA-Z0-9_]*\b' internal/contracts/fields.go internal/store/sqlite/adapter.go | \
        grep -E '^(order|customer|shipment|payment|return|refund)' | \
        sort | uniq -c | sort -rn | head -20

    echo ""

    # Enum values
    print_info "Enum values found:"
    grep -r 'EnumValues:.*\[\]string{' internal/semantic/schema_provider.go -A1 | \
        grep -o '"[^"]*"' | sort | uniq

    echo ""

    # Identifier patterns
    print_info "Identifier patterns:"
    grep -r 'regexp.MustCompile' internal/identifier/detector.go | \
        grep -o '`[^`]*`'

    echo ""
    print_success "Analysis complete"
}

# Function to create backup
create_backup() {
    local timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_dir="backups/schema_migration_$timestamp"

    print_info "Creating backup..."

    mkdir -p "$backup_dir"

    # Backup critical files
    cp -r internal/contracts "$backup_dir/"
    cp -r internal/identifier "$backup_dir/"
    cp -r internal/semantic "$backup_dir/"
    cp -r internal/store "$backup_dir/"
    cp -r migrations "$backup_dir/"

    # Backup database if exists
    if [ -f "data/search.db" ]; then
        cp data/search.db "$backup_dir/search.db.backup"
    fi

    print_success "Backup created: $backup_dir"
    echo ""
    print_info "To restore from backup:"
    echo "  cp -r $backup_dir/* ./"
}

# Main script logic
main() {
    local command=$1

    case $command in
        find)
            if [ -z "$2" ]; then
                print_error "Error: Pattern required"
                echo "Usage: ./migrate_schema.sh find <pattern>"
                exit 1
            fi
            find_pattern "$2"
            ;;

        replace)
            if [ -z "$2" ] || [ -z "$3" ]; then
                print_error "Error: Both old and new values required"
                echo "Usage: ./migrate_schema.sh replace <old> <new>"
                exit 1
            fi
            perform_replacement "$2" "$3"
            ;;

        preview)
            if [ -z "$2" ] || [ -z "$3" ]; then
                print_error "Error: Both old and new values required"
                echo "Usage: ./migrate_schema.sh preview <old> <new>"
                exit 1
            fi
            preview_replacement "$2" "$3"
            ;;

        analyze)
            analyze_schema
            ;;

        backup)
            create_backup
            ;;

        help|--help|-h)
            show_usage
            ;;

        *)
            print_error "Unknown command: $command"
            echo ""
            show_usage
            exit 1
            ;;
    esac
}

# Check if running from project root
if [ ! -d "internal" ] || [ ! -d "migrations" ]; then
    print_error "Error: Must run from project root directory"
    exit 1
fi

# Run main
main "$@"
