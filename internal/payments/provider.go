package payments

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrMalformedPayload       = errors.New("malformed payment payload")
	ErrAmountCurrencyMismatch = errors.New("amount or currency mismatch")
	ErrMissingChargeID        = errors.New("missing charge id")
)

type SuccessfulPaymentInput struct {
	Currency         string
	AmountCents      int64
	ProviderChargeID string
	TelegramChargeID string
}

func ValidateSuccessfulPayment(input SuccessfulPaymentInput, expectedCurrency string, expectedAmountCents int64) error {
	if strings.TrimSpace(input.Currency) == "" || input.AmountCents <= 0 {
		return ErrMalformedPayload
	}
	if !strings.EqualFold(strings.TrimSpace(input.Currency), strings.TrimSpace(expectedCurrency)) || input.AmountCents != expectedAmountCents {
		return ErrAmountCurrencyMismatch
	}
	if strings.TrimSpace(input.ProviderChargeID) == "" || strings.TrimSpace(input.TelegramChargeID) == "" {
		return ErrMissingChargeID
	}
	return nil
}

func BuildStarsOperationID(providerChargeID, telegramChargeID string) (string, error) {
	providerChargeID = strings.TrimSpace(providerChargeID)
	telegramChargeID = strings.TrimSpace(telegramChargeID)
	if providerChargeID == "" || telegramChargeID == "" {
		return "", fmt.Errorf("%w: provider=%q telegram=%q", ErrMissingChargeID, providerChargeID, telegramChargeID)
	}
	return "tg_stars:" + providerChargeID + ":" + telegramChargeID, nil
}
