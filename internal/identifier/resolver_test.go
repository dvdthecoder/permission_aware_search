package identifier

import "testing"

func TestBuildResolutionPlanForOrderAndTracking(t *testing.T) {
	p := BuildResolutionPlan("ORD-000123 TRK-00001234", "")
	if !p.ShouldUseFastPath {
		t.Fatalf("expected fast path")
	}
	if len(p.Groups) < 2 {
		t.Fatalf("expected grouped specs, got %d", len(p.Groups))
	}
}

func TestBuildResolutionPlanUnknownTokenIncludesParallelGroups(t *testing.T) {
	p := BuildResolutionPlan("X-UNKNOWN-77", "")
	if !p.ShouldUseFastPath {
		t.Fatalf("expected fast path for token-like input")
	}
	if len(p.Groups) < 4 {
		t.Fatalf("expected parallel groups for unknown token, got %d", len(p.Groups))
	}
}

func TestBuildResolutionPlanTypeaheadUsesLike(t *testing.T) {
	reg := LoadPatternRegistry("")
	p := BuildResolutionPlanWithConfig("pix-gj", "tenant-a", "", reg, QueryShapeThresholds{
		ShortNoOpLen:        2,
		GenericPrefixMinLen: 3,
		IDPrefixMinLen:      2,
		EmailPrefixMinLen:   3,
	})
	if p.QueryShape != ShapeTypeahead && p.QueryShape != ShapeIdentifier {
		t.Fatalf("expected typeahead or identifier shape, got %s", p.QueryShape)
	}
	foundLike := false
	for _, g := range p.Groups {
		if g.Operator == "like" {
			foundLike = true
			break
		}
	}
	if !foundLike {
		t.Fatalf("expected at least one like operator for typeahead resolution")
	}
}
