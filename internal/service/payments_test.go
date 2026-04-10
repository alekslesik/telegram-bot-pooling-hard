package service

import (
	"context"
	"testing"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestPaymentPayloadV1_RoundTrip(t *testing.T) {
	encoded, err := EncodePaymentPayloadV1(PaymentPayloadV1{
		Version: 1,
		UserID:  101,
		Stars:   25,
	})
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	decoded, err := DecodePaymentPayloadV1(encoded)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if decoded.Version != 1 || decoded.UserID != 101 || decoded.Stars != 25 {
		t.Fatalf("unexpected payload after round-trip: %+v", decoded)
	}
}

func TestPaymentPayloadV1_InvalidPayload(t *testing.T) {
	if _, err := DecodePaymentPayloadV1("not-base64"); err == nil {
		t.Fatal("expected decode error for invalid payload")
	}
}

func TestPaymentService_ValidatePreCheckout_WrongUser(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewPaymentService(repo)

	payload, err := svc.BuildTopUpInvoicePayload(2001, 7)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if err := svc.ValidatePreCheckout(2002, "XTR", 7, payload); err == nil {
		t.Fatal("expected wrong-user validation error")
	}
}

func TestPaymentService_ValidatePreCheckout_WrongAmount(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewPaymentService(repo)

	payload, err := svc.BuildTopUpInvoicePayload(2010, 7)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if err := svc.ValidatePreCheckout(2010, "XTR", 8, payload); err == nil {
		t.Fatal("expected wrong-amount validation error")
	}
}

func TestPaymentService_ValidatePreCheckout_WrongCurrency(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewPaymentService(repo)

	payload, err := svc.BuildTopUpInvoicePayload(2020, 7)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if err := svc.ValidatePreCheckout(2020, "USD", 7, payload); err == nil {
		t.Fatal("expected wrong-currency validation error")
	}
}

func TestPaymentService_ApplySuccessfulPayment_MemoryRepository(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewPaymentService(repo)
	ctx := context.Background()
	const userID int64 = 2030

	before, err := repo.EnsureUserProfile(ctx, userID)
	if err != nil {
		t.Fatalf("ensure profile: %v", err)
	}

	payload, err := svc.BuildTopUpInvoicePayload(userID, 9)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	result, err := svc.ApplySuccessfulPayment(userID, &tgbotapi.SuccessfulPayment{
		Currency:                "XTR",
		TotalAmount:             9,
		InvoicePayload:          payload,
		TelegramPaymentChargeID: "tg-charge-2030",
		ProviderPaymentChargeID: "provider-2030",
	})
	if err != nil {
		t.Fatalf("apply successful payment: %v", err)
	}
	if result.AlreadyApplied {
		t.Fatal("first payment apply should not be marked as already applied")
	}
	if result.CreditedCents != 900 {
		t.Fatalf("expected 900 credited cents, got %d", result.CreditedCents)
	}
	if result.BalanceAfter != before.BalanceCents+900 {
		t.Fatalf("unexpected balance after payment: got=%d want=%d", result.BalanceAfter, before.BalanceCents+900)
	}

	replayed, err := svc.ApplySuccessfulPayment(userID, &tgbotapi.SuccessfulPayment{
		Currency:                "XTR",
		TotalAmount:             9,
		InvoicePayload:          payload,
		TelegramPaymentChargeID: "tg-charge-2030",
		ProviderPaymentChargeID: "provider-2030",
	})
	if err != nil {
		t.Fatalf("apply idempotent payment: %v", err)
	}
	if !replayed.AlreadyApplied {
		t.Fatal("replayed payment must be idempotent")
	}
	if replayed.BalanceAfter != result.BalanceAfter {
		t.Fatalf("idempotent replay changed balance: first=%d second=%d", result.BalanceAfter, replayed.BalanceAfter)
	}
}
