package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	paymentPayloadVersionV1 int64  = 1
	paymentPayloadMaxBytes  int    = 128
	paymentCurrencyXTR      string = "XTR"
	starsKopeksPerUnit      int64  = 100
)

type PaymentPayloadV1 struct {
	Version int64 `json:"v"`
	UserID  int64 `json:"u"`
	Stars   int64 `json:"s"`
}

func EncodePaymentPayloadV1(payload PaymentPayloadV1) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if len(encoded) > paymentPayloadMaxBytes {
		return "", fmt.Errorf("payload exceeds %d bytes", paymentPayloadMaxBytes)
	}
	return encoded, nil
}

func DecodePaymentPayloadV1(payload string) (PaymentPayloadV1, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(payload))
	if err != nil {
		return PaymentPayloadV1{}, fmt.Errorf("decode payload: %w", err)
	}
	var out PaymentPayloadV1
	if err := json.Unmarshal(raw, &out); err != nil {
		return PaymentPayloadV1{}, fmt.Errorf("unmarshal payload: %w", err)
	}
	if out.Version != paymentPayloadVersionV1 {
		return PaymentPayloadV1{}, fmt.Errorf("unsupported payload version")
	}
	if out.UserID <= 0 || out.Stars <= 0 {
		return PaymentPayloadV1{}, fmt.Errorf("invalid payload values")
	}
	return out, nil
}

type PaymentService struct {
	repo repository.BookingRepository
}

func NewPaymentService(repo repository.BookingRepository) *PaymentService {
	return &PaymentService{repo: repo}
}

func (s *PaymentService) BuildTopUpInvoicePayload(userID, stars int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("user id must be positive")
	}
	if stars <= 0 {
		return "", fmt.Errorf("stars must be positive")
	}
	return EncodePaymentPayloadV1(PaymentPayloadV1{
		Version: paymentPayloadVersionV1,
		UserID:  userID,
		Stars:   stars,
	})
}

func (s *PaymentService) ValidatePreCheckout(fromUserID int64, currency string, totalStars int64, payload string) error {
	if strings.TrimSpace(currency) != paymentCurrencyXTR {
		return fmt.Errorf("unsupported currency")
	}
	decoded, err := DecodePaymentPayloadV1(payload)
	if err != nil {
		return err
	}
	if decoded.UserID != fromUserID {
		return fmt.Errorf("payload user mismatch")
	}
	if decoded.Stars != totalStars {
		return fmt.Errorf("payload stars mismatch")
	}
	return nil
}

func (s *PaymentService) ApplySuccessfulPayment(fromUserID int64, sp *tgbotapi.SuccessfulPayment) (repository.StarsTopUpResult, error) {
	if sp == nil {
		return repository.StarsTopUpResult{}, fmt.Errorf("successful payment is required")
	}
	totalStars := int64(sp.TotalAmount)
	if err := s.ValidatePreCheckout(fromUserID, sp.Currency, totalStars, sp.InvoicePayload); err != nil {
		return repository.StarsTopUpResult{}, err
	}
	decoded, err := DecodePaymentPayloadV1(sp.InvoicePayload)
	if err != nil {
		return repository.StarsTopUpResult{}, err
	}
	chargeID := strings.TrimSpace(sp.TelegramPaymentChargeID)
	if chargeID == "" {
		return repository.StarsTopUpResult{}, fmt.Errorf("telegram payment charge id is required")
	}
	metaRaw, _ := json.Marshal(map[string]any{
		"v":                          decoded.Version,
		"currency":                   sp.Currency,
		"stars":                      decoded.Stars,
		"telegram_payment_charge_id": chargeID,
		"provider_payment_charge_id": strings.TrimSpace(sp.ProviderPaymentChargeID),
	})

	return s.repo.ApplyTelegramStarsTopUp(
		context.Background(),
		fromUserID,
		decoded.Stars,
		starsKopeksPerUnit,
		chargeID,
		string(metaRaw),
	)
}
