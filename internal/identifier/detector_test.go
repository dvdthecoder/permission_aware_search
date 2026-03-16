package identifier

import "testing"

func TestDetectCommonIdentifiers(t *testing.T) {
	cases := []struct {
		in      string
		wantTyp IdentifierType
	}{
		{"ORD-000123", TypeOrderNumber},
		{"TRK-00001234", TypeTrackingID},
		{"PAY-00001234", TypePaymentRef},
		{"CUST-000123", TypeCustomerNumber},
		{"aster@example.com", TypeEmail},
		{"+1 415 555 1212", TypePhone},
	}
	for _, tc := range cases {
		out := Detect(tc.in)
		if len(out) == 0 {
			t.Fatalf("expected detection for %q", tc.in)
		}
		if out[0].Type != tc.wantTyp {
			t.Fatalf("expected %s for %q got %s", tc.wantTyp, tc.in, out[0].Type)
		}
	}
}

func TestShouldUseFastPath(t *testing.T) {
	if !ShouldUseFastPath("ORD-000123", Detect("ORD-000123")) {
		t.Fatalf("expected fast path for raw id")
	}
	if ShouldUseFastPath("show open orders this week", Detect("show open orders this week")) {
		t.Fatalf("did not expect fast path for sentence query")
	}
}
