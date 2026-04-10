package payments

import (
	"errors"
	"testing"
)

func TestValidateSuccessfulPayment_MalformedPayload(t *testing.T) {
	err := ValidateSuccessfulPayment(SuccessfulPaymentInput{}, "XTR", 100)
	if !errors.Is(err, ErrMalformedPayload) {
		t.Fatalf("expected ErrMalformedPayload, got %v", err)
	}
}

func TestValidateSuccessfulPayment_MismatchedAmountOrCurrency(t *testing.T) {
	err := ValidateSuccessfulPayment(SuccessfulPaymentInput{
		Currency:         "USD",
		AmountCents:      200,
		ProviderChargeID: "provider-1",
		TelegramChargeID: "tg-1",
	}, "XTR", 100)
	if !errors.Is(err, ErrAmountCurrencyMismatch) {
		t.Fatalf("expected ErrAmountCurrencyMismatch, got %v", err)
	}
}

func TestValidateSuccessfulPayment_MissingChargeID(t *testing.T) {
	err := ValidateSuccessfulPayment(SuccessfulPaymentInput{
		Currency:         "XTR",
		AmountCents:      100,
		ProviderChargeID: "",
		TelegramChargeID: "tg-1",
	}, "XTR", 100)
	if !errors.Is(err, ErrMissingChargeID) {
		t.Fatalf("expected ErrMissingChargeID, got %v", err)
	}
}

func TestBuildStarsOperationID(t *testing.T) {
	got, err := BuildStarsOperationID("provider-1", "tg-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "tg_stars:provider-1:tg-1" {
		t.Fatalf("unexpected operation id: %s", got)
	}
}
