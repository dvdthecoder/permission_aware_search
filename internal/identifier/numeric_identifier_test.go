package identifier

import (
	"testing"
)

func TestNumericOnlyIdentifierDetection(t *testing.T) {
	tests := []struct {
		input              string
		expectedDetected   bool
		expectedType       IdentifierType
		description        string
	}{
		{
			input:            "1004",
			expectedDetected: true,
			expectedType:     TypeUnknownToken,
			description:      "Pure numeric identifier",
		},
		{
			input:            "12345",
			expectedDetected: true,
			expectedType:     TypeUnknownToken,
			description:      "5-digit numeric",
		},
		{
			input:            "123456",
			expectedDetected: true,
			expectedType:     TypeOrderNumber, // Detected as order_number by numeric_long pattern
			description:      "6-digit numeric (detected as order)",
		},
		{
			input:            "123",
			expectedDetected: true,
			expectedType:     TypeUnknownToken,
			description:      "Short 3-digit numeric",
		},
		{
			input:            "ord1004",
			expectedDetected: true,
			expectedType:     TypeUnknownToken,
			description:      "Alphanumeric without separator",
		},
		{
			input:            "ORD-001234",
			expectedDetected: true,
			expectedType:     TypeOrderNumber,
			description:      "Full order number format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			detected := Detect(tt.input)

			if tt.expectedDetected && len(detected) == 0 {
				t.Errorf("Input '%s': expected detection, got none", tt.input)
			}

			if !tt.expectedDetected && len(detected) > 0 {
				t.Errorf("Input '%s': expected no detection, got %d", tt.input, len(detected))
			}

			if tt.expectedDetected && len(detected) > 0 {
				if detected[0].Type != tt.expectedType {
					t.Errorf("Input '%s': expected type %s, got %s",
						tt.input, tt.expectedType, detected[0].Type)
				}

				if detected[0].NormalizedValue == "" {
					t.Errorf("Input '%s': normalized value is empty", tt.input)
				}
			}
		})
	}
}

func TestNumericIdentifierResolutionPlan(t *testing.T) {
	tests := []struct {
		input               string
		shouldUseFastPath   bool
		expectedGroupCount  int
		description         string
	}{
		{
			input:              "1004",
			shouldUseFastPath:  true,
			expectedGroupCount: 4, // order.number, tracking_id, payment.reference, customer.number
			description:        "Numeric should create multi-field search plan",
		},
		{
			input:              "123456",
			shouldUseFastPath:  true,
			expectedGroupCount: 4,
			description:        "6-digit numeric (common order pattern)",
		},
		{
			input:              "ord1004",
			shouldUseFastPath:  true,
			expectedGroupCount: 5,
			description:        "Alphanumeric partial",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			plan := BuildResolutionPlan(tt.input, "")

			if plan.ShouldUseFastPath != tt.shouldUseFastPath {
				t.Errorf("Input '%s': expected shouldUseFastPath=%v, got %v",
					tt.input, tt.shouldUseFastPath, plan.ShouldUseFastPath)
			}

			if len(plan.Groups) != tt.expectedGroupCount {
				t.Errorf("Input '%s': expected %d groups, got %d. Groups: %+v",
					tt.input, tt.expectedGroupCount, len(plan.Groups), plan.Groups)
			}

			// Verify all groups use LIKE operator for prefix matching
			for _, group := range plan.Groups {
				if group.Operator != "like" {
					t.Errorf("Input '%s': expected LIKE operator for group %s.%s, got %s",
						tt.input, group.ResourceType, group.MatchField, group.Operator)
				}

				// Verify value ends with % for prefix matching
				expectedValue := tt.input + "%"
				if group.Value != expectedValue {
					t.Errorf("Input '%s': expected value '%s', got '%s'",
						tt.input, expectedValue, group.Value)
				}
			}
		})
	}
}

func TestQueryShapeForNumericInput(t *testing.T) {
	tests := []struct {
		input         string
		expectedShape QueryShape
		description   string
	}{
		{
			input:         "1",
			expectedShape: ShapeTypeahead,
			description:   "Single digit too short",
		},
		{
			input:         "12",
			expectedShape: ShapeTypeahead,
			description:   "Two digits too short",
		},
		{
			input:         "123",
			expectedShape: ShapeTypeahead,
			description:   "Three digits - valid typeahead",
		},
		{
			input:         "1004",
			expectedShape: ShapeTypeahead,
			description:   "Four digits - valid typeahead",
		},
		{
			input:         "123456",
			expectedShape: ShapeIdentifier, // Detected as order number, but will use prefix matching
			description:   "Six digits - detected as identifier",
		},
		{
			input:         "ORD-123456",
			expectedShape: ShapeIdentifier,
			description:   "Full order format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			analysis := AnalyzeQuery(tt.input, "", nil, QueryShapeThresholds{})

			if analysis.QueryShape != tt.expectedShape {
				t.Errorf("Input '%s': expected shape %s, got %s",
					tt.input, tt.expectedShape, analysis.QueryShape)
			}
		})
	}
}

func TestNumericIdentifierNormalization(t *testing.T) {
	tests := []struct {
		input      string
		expected   string
		description string
	}{
		{
			input:      "  1004  ",
			expected:   "1004",
			description: "Trim whitespace",
		},
		{
			input:      "1004",
			expected:   "1004",
			description: "Already normalized",
		},
		{
			input:      "'1004'",
			expected:   "1004",
			description: "Remove quotes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			normalized, _ := NormalizeInput(tt.input)

			if normalized != tt.expected {
				t.Errorf("Input '%s': expected normalized '%s', got '%s'",
					tt.input, tt.expected, normalized)
			}
		})
	}
}
