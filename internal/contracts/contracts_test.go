package contracts

import "testing"

func TestValidateFieldV1(t *testing.T) {
	if err := ValidateField("order", ContractVersionV1, "", "status"); err != nil {
		t.Fatalf("expected field to be allowed, got %v", err)
	}
	if err := ValidateField("order", ContractVersionV1, "", "order_number"); err == nil {
		t.Fatalf("expected disallowed field error in v1")
	}
}

func TestValidateFieldV2IntentScoped(t *testing.T) {
	if err := ValidateField("order", ContractVersionV2, IntentWISMO, "tracking_id"); err != nil {
		t.Fatalf("expected tracking_id in wismo allowlist, got %v", err)
	}
	if err := ValidateField("order", ContractVersionV2, IntentWISMO, "return_status"); err == nil {
		t.Fatalf("expected return_status to be disallowed in wismo intent")
	}
	if err := ValidateField("order", ContractVersionV2, IntentReturnsRefunds, "return_status"); err != nil {
		t.Fatalf("expected return_status in returns_refunds allowlist, got %v", err)
	}
	if err := ValidateField("order", ContractVersionV2, IntentWISMO, "payment_reference"); err != nil {
		t.Fatalf("expected payment_reference in wismo allowlist, got %v", err)
	}
	if err := ValidateField("order", ContractVersionV2, IntentReturnsRefunds, "payment_reference"); err != nil {
		t.Fatalf("expected payment_reference in returns_refunds allowlist, got %v", err)
	}
}
