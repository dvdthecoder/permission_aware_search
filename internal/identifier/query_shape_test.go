package identifier

import "testing"

func TestAnalyzeQueryShapesRealExamples(t *testing.T) {
	reg := LoadPatternRegistry("")
	thresholds := QueryShapeThresholds{ShortNoOpLen: 2, GenericPrefixMinLen: 3, IDPrefixMinLen: 2, EmailPrefixMinLen: 3}

	cases := []struct {
		in   string
		want QueryShape
	}{
		{"PIX-GJV-287770", ShapeIdentifier},
		{"x14881125", ShapeIdentifier},
		{"f260201", ShapeIdentifier},
		{"7823419", ShapeIdentifier},
		{"zora.ivan@gmail.com", ShapeContact},
		{"voctesuk@gmaiul", ShapeTypeahead},
		{"° PIX-GJV-287770", ShapeIdentifier},
		{"x", ShapeTypeahead},
		{"show open orders this week", ShapeSentence},
	}

	for _, tc := range cases {
		got := AnalyzeQuery(tc.in, "tenant-a", reg, thresholds)
		if got.QueryShape != tc.want {
			t.Fatalf("input=%q expected=%s got=%s", tc.in, tc.want, got.QueryShape)
		}
	}
}

func TestNormalizeInputAppliesWrappersAndNoise(t *testing.T) {
	in := `index: pixart_ecomm01prod84_lineitems, searchQuery: ° PIX-GJV-287770`
	got, applied := NormalizeInput(in)
	if got != "PIX-GJV-287770" {
		t.Fatalf("expected normalized query PIX-GJV-287770, got %q", got)
	}
	if len(applied) == 0 {
		t.Fatalf("expected normalizationApplied entries")
	}
}
