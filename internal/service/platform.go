package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
)

// InsufficientFundsError is returned when a paid booking cannot cover the fee.
type InsufficientFundsError struct {
	FeeCents     int64
	BalanceCents int64
}

func (e *InsufficientFundsError) Error() string {
	return "insufficient funds"
}

// Demo economy (Level 3 RFC): internal credits, not real money.
const (
	ClinicBookingFeeCents    int64 = 100
	RefereeSignupBonusCents  int64 = 50
	ReferrerSignupBonusCents int64 = 100
	specialtyCacheTTL              = 30 * time.Second
)

// SpecialtyPageCache caches paginated specialty list JSON (optional).
type SpecialtyPageCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, val string, ttl time.Duration) error
}

// EnsureUserStart creates a user profile and applies a referral code from /start payload.
func (s *BookingService) EnsureUserStart(ctx context.Context, userID int64, startPayload string) error {
	if _, err := s.repo.EnsureUserProfile(ctx, userID); err != nil {
		return err
	}
	code := strings.TrimSpace(strings.ToLower(startPayload))
	code = strings.TrimPrefix(code, "ref_")
	return s.repo.ApplyReferralCodeIfNew(ctx, userID, code)
}

func (s *BookingService) LogAnalytics(ctx context.Context, userID *int64, eventType, payloadJSON string) error {
	return s.repo.LogAnalyticsEvent(ctx, userID, eventType, payloadJSON)
}

func (s *BookingService) PreferredLang(ctx context.Context, userID int64) (string, error) {
	p, err := s.repo.GetUserProfile(ctx, userID)
	if err != nil {
		if err == repository.ErrNotFound {
			return "ru", nil
		}
		return "ru", err
	}
	return p.PreferredLang, nil
}

func (s *BookingService) SetPreferredLang(ctx context.Context, userID int64, lang string) error {
	if _, err := s.repo.EnsureUserProfile(ctx, userID); err != nil {
		return err
	}
	return s.repo.SetPreferredLang(ctx, userID, lang)
}

func (s *BookingService) AccountCabinet(ctx context.Context, userID int64) (balanceCents int64, referralCode string, err error) {
	p, err := s.repo.EnsureUserProfile(ctx, userID)
	if err != nil {
		return 0, "", err
	}
	return p.BalanceCents, p.ReferralCode, nil
}

func (s *BookingService) AdminAnalyticsReport(ctx context.Context, adminUserID int64) (string, error) {
	ok, err := s.repo.IsAdmin(ctx, adminUserID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("not admin")
	}
	since := time.Now().UTC().Add(-7 * 24 * time.Hour)
	counts, err := s.repo.CountAnalyticsByEventSince(ctx, since)
	if err != nil {
		return "", err
	}
	if len(counts) == 0 {
		return "", nil
	}
	var lines []string
	for k, v := range counts {
		lines = append(lines, fmt.Sprintf("- %s: %d", k, v))
	}
	return strings.Join(lines, "\n"), nil
}
