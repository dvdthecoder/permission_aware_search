package semantic

import (
	"strings"
	"testing"

	"permission_aware_search/internal/contracts"
)

func TestTemporalQueriesWithCreated(t *testing.T) {
	tests := []struct {
		query                string
		expectedIntent       string
		shouldHaveTimeFilter bool
		description          string
	}{
		{
			query:                "orders created this week",
			expectedIntent:       contracts.IntentWISMO,
			shouldHaveTimeFilter: true,
			description:          "Simple temporal query with 'created'",
		},
		{
			query:                "orders created this month",
			expectedIntent:       contracts.IntentWISMO,
			shouldHaveTimeFilter: true,
			description:          "Monthly temporal query with 'created'",
		},
		{
			query:                "show orders created this week",
			expectedIntent:       contracts.IntentWISMO,
			shouldHaveTimeFilter: true,
			description:          "Temporal query with 'show' prefix",
		},
		{
			query:                "list orders created last 30 days",
			expectedIntent:       contracts.IntentWISMO,
			shouldHaveTimeFilter: true,
			description:          "Temporal query with 'last 30 days'",
		},
		{
			query:                "orders this week",
			expectedIntent:       contracts.IntentWISMO,
			shouldHaveTimeFilter: true,
			description:          "Short form without 'created'",
		},
		{
			query:                "orders for the week",
			expectedIntent:       contracts.IntentWISMO,
			shouldHaveTimeFilter: true,
			description:          "Alternative phrasing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := ParseNaturalLanguage(tt.query, contracts.ContractVersionV2, "order")

			// Check intent
			if result.IntentCategory != tt.expectedIntent {
				t.Errorf("Query '%s': expected intent %s, got %s", tt.query, tt.expectedIntent, result.IntentCategory)
			}

			// Check for time filter
			hasTimeFilter := false
			for _, filter := range result.Query.Filters {
				if strings.Contains(filter.Field, "created_at") {
					hasTimeFilter = true
					break
				}
			}

			if tt.shouldHaveTimeFilter && !hasTimeFilter {
				t.Errorf("Query '%s': expected created_at filter, but none found. Filters: %+v", tt.query, result.Query.Filters)
			}

			// Check evidence
			if tt.shouldHaveTimeFilter {
				hasTimeEvidence := false
				for _, evidence := range result.SafeEvidence {
					if strings.Contains(evidence, "time_window") {
						hasTimeEvidence = true
						break
					}
				}
				if !hasTimeEvidence {
					t.Errorf("Query '%s': expected time_window evidence, got: %v", tt.query, result.SafeEvidence)
				}
			}
		})
	}
}

func TestTemporalWindowParsing(t *testing.T) {
	tests := []struct {
		input        string
		expectedDays int
		expectedOk   bool
	}{
		// Week patterns
		{"orders this week", 7, true},
		{"orders for the week", 7, true},
		{"orders for week", 7, true},
		{"orders created this week", 7, true},
		{"show orders created this week", 7, true},
		{"last 7 days orders", 7, true},

		// Month patterns
		{"orders this month", 30, true},
		{"orders for the month", 30, true},
		{"orders for month", 30, true},
		{"orders created this month", 30, true},
		{"show orders created this month", 30, true},
		{"orders last 30 days", 30, true},
		{"orders created last 30 days", 30, true},

		// Not temporal
		{"orders today", 0, false},
		{"random query", 0, false},
		{"order status", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lower := strings.ToLower(tt.input)
			days, _, ok := inferRelativeWindow(lower)

			if ok != tt.expectedOk {
				t.Errorf("Input '%s': expected ok=%v, got ok=%v", tt.input, tt.expectedOk, ok)
			}

			if ok && days != tt.expectedDays {
				t.Errorf("Input '%s': expected %d days, got %d days", tt.input, tt.expectedDays, days)
			}
		})
	}
}

func TestTemporalIntentClassification(t *testing.T) {
	tests := []struct {
		query          string
		expectedIntent string
	}{
		{"orders created this week", contracts.IntentWISMO},
		{"orders created this month", contracts.IntentWISMO},
		{"show orders for the week", contracts.IntentWISMO},
		{"list orders last 30 days", contracts.IntentWISMO},
		{"orders this week", contracts.IntentWISMO},
		{"orders for the month", contracts.IntentWISMO},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := ParseNaturalLanguage(tt.query, contracts.ContractVersionV2, "order")

			if result.IntentCategory != tt.expectedIntent {
				t.Errorf("Query '%s': expected intent %s, got %s",
					tt.query, tt.expectedIntent, result.IntentCategory)
			}
		})
	}
}
