package service

import (
	"context"
	"testing"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/payments"
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

func TestPaymentService_ApplyExternalProviderCallback_Validation(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewPaymentService(repo)

	_, err := svc.ApplyExternalProviderCallback(payments.ExternalCallback{
		ProviderCode: "Bad!",
		OperationRef: "x",
		UserID:       0,
		AmountMinor:  0,
		Currency:     "",
	})
	if err == nil {
		t.Fatal("expected validation error for invalid external callback")
	}
}

func TestPaymentService_ApplyExternalProviderCallback_Idempotent(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewPaymentService(repo)
	ctx := context.Background()
	const userID int64 = 3040

	before, err := repo.EnsureUserProfile(ctx, userID)
	if err != nil {
		t.Fatalf("ensure profile: %v", err)
	}
	callback := payments.ExternalCallback{
		ProviderCode: "yookassa",
		OperationRef: "pay_3040_attempt_1",
		UserID:       userID,
		AmountMinor:  1599,
		Currency:     "RUB",
	}

	first, err := svc.ApplyExternalProviderCallback(callback)
	if err != nil {
		t.Fatalf("apply external callback: %v", err)
	}
	if first.AlreadyApplied {
		t.Fatal("first external callback apply should not be marked as already applied")
	}
	if first.CreditedCents != callback.AmountMinor {
		t.Fatalf("unexpected credited cents: got=%d want=%d", first.CreditedCents, callback.AmountMinor)
	}
	if first.BalanceAfter != before.BalanceCents+callback.AmountMinor {
		t.Fatalf("unexpected balance after first callback: got=%d want=%d", first.BalanceAfter, before.BalanceCents+callback.AmountMinor)
	}

	second, err := svc.ApplyExternalProviderCallback(callback)
	if err != nil {
		t.Fatalf("apply duplicated external callback: %v", err)
	}
	if !second.AlreadyApplied {
		t.Fatal("duplicated external callback must be idempotent")
	}
	if second.BalanceAfter != first.BalanceAfter {
		t.Fatalf("duplicated callback changed balance: first=%d second=%d", first.BalanceAfter, second.BalanceAfter)
	}
}

func TestPaymentService_ApplySuccessfulPayment_RepeatedDeliveries_FinalBalanceIdempotent(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewPaymentService(repo)
	ctx := context.Background()
	const userID int64 = 2040

	before, err := repo.EnsureUserProfile(ctx, userID)
	if err != nil {
		t.Fatalf("ensure profile: %v", err)
	}
	payload, err := svc.BuildTopUpInvoicePayload(userID, 12)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}

	var firstBalance int64
	for i := 0; i < 5; i++ {
		res, err := svc.ApplySuccessfulPayment(userID, &tgbotapi.SuccessfulPayment{
			Currency:                "XTR",
			TotalAmount:             12,
			InvoicePayload:          payload,
			TelegramPaymentChargeID: "tg-charge-2040-repeat",
			ProviderPaymentChargeID: "provider-2040-repeat",
		})
		if err != nil {
			t.Fatalf("apply delivery #%d failed: %v", i+1, err)
		}
		if i == 0 {
			if res.AlreadyApplied {
				t.Fatal("first delivery must not be marked as already applied")
			}
			firstBalance = res.BalanceAfter
			continue
		}
		if !res.AlreadyApplied {
			t.Fatalf("delivery #%d must be idempotent", i+1)
		}
		if res.BalanceAfter != firstBalance {
			t.Fatalf("delivery #%d changed balance: got=%d want=%d", i+1, res.BalanceAfter, firstBalance)
		}
	}

	after, err := repo.GetUserProfile(ctx, userID)
	if err != nil {
		t.Fatalf("get profile after replays: %v", err)
	}
	want := before.BalanceCents + 1200
	if after.BalanceCents != want {
		t.Fatalf("unexpected final balance after repeated deliveries: got=%d want=%d", after.BalanceCents, want)
	}
}
