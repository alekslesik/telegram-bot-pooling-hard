package payments

import "testing"

func TestExternalCallbackValidate(t *testing.T) {
	ok := ExternalCallback{
		ProviderCode: "yookassa",
		OperationRef: "pay_123:attempt_1",
		UserID:       42,
		AmountMinor:  1500,
		Currency:     "RUB",
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid callback rejected: %v", err)
	}

	cases := []ExternalCallback{
		{ProviderCode: "Bad!", OperationRef: "pay_1", UserID: 1, AmountMinor: 1, Currency: "RUB"},
		{ProviderCode: "yookassa", OperationRef: "x", UserID: 1, AmountMinor: 1, Currency: "RUB"},
		{ProviderCode: "yookassa", OperationRef: "pay_1", UserID: 0, AmountMinor: 1, Currency: "RUB"},
		{ProviderCode: "yookassa", OperationRef: "pay_1", UserID: 1, AmountMinor: 0, Currency: "RUB"},
		{ProviderCode: "yookassa", OperationRef: "pay_1", UserID: 1, AmountMinor: 1, Currency: ""},
	}
	for i, tc := range cases {
		if err := tc.Validate(); err == nil {
			t.Fatalf("case %d expected validation error", i)
		}
	}
}

func TestBuildExternalOperationID(t *testing.T) {
	got, err := BuildExternalOperationID("yookassa", "pay_123:attempt_1")
	if err != nil {
		t.Fatalf("build operation id: %v", err)
	}
	want := "external_psp:yookassa:pay_123:attempt_1"
	if got != want {
		t.Fatalf("unexpected operation id: got=%q want=%q", got, want)
	}
}
