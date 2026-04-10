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
	return logAnalyticsWithEnvelope(ctx, s.repo, userID, eventType, payloadJSON, "bot")
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

func (s *BookingService) AdminAnalyticsReport(ctx context.Context, adminUserID int64, days int, specialtyID *int64) (string, error) {
	caps, err := s.AdminCapabilities(ctx, adminUserID)
	if err != nil {
		return "", err
	}
	if !caps.CanViewAnalytics {
		return "", fmt.Errorf("not admin")
	}
	if days <= 0 {
		days = 7
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	counts, err := s.repo.CountAnalyticsByEventSince(ctx, since)
	if err != nil {
		return "", err
	}
	confirmedBookings, err := s.repo.CountBookingsConfirmedSinceWithOptionalSpecialty(ctx, since, specialtyID)
	if err != nil {
		return "", err
	}
	cancellations, err := s.repo.CountClinicBookingsCancelledSince(ctx, since)
	if err != nil {
		return "", err
	}
	noShowProxy, err := s.repo.CountNoShowProxySince(ctx, since)
	if err != nil {
		return "", err
	}
	referralRewardsGranted, err := s.repo.CountReferralRewardsGrantedSince(ctx, since)
	if err != nil {
		return "", err
	}
	retentionUsers, err := s.repo.CountRetentionUsersSince(ctx, since)
	if err != nil {
		return "", err
	}
	outbox, err := s.repo.CountOutboxByStatus(ctx)
	if err != nil {
		return "", err
	}
	ops, err := s.repo.GetOutboxOperationalStats(ctx)
	if err != nil {
		return "", err
	}
	walletMismatches, err := s.repo.CountWalletBalanceMismatches(ctx)
	if err != nil {
		return "", err
	}
	segment := "all"
	if specialtyID != nil {
		segment = fmt.Sprintf("specialty_id=%d", *specialtyID)
	}
	specSelected := counts["funnel_book_specialty_selected"]
	conversion := "0.00"
	if specSelected > 0 {
		conversion = fmt.Sprintf("%.2f", float64(confirmedBookings)/float64(specSelected))
	}
	funnelTail := []string{
		fmt.Sprintf("- period: last %d days", days),
		fmt.Sprintf("- segment: %s", segment),
		fmt.Sprintf("- funnel_conversion_approx_confirmed_per_specialty_selected: %s", conversion),
		"- funnel_conversion_note: count-based_not_unique_users",
		fmt.Sprintf("- bookings_confirmed_total: %d", confirmedBookings),
		fmt.Sprintf("- cancellations: %d", cancellations),
		fmt.Sprintf("- no_show_proxy: %d", noShowProxy),
		fmt.Sprintf("- referral_rewards_granted: %d", referralRewardsGranted),
		fmt.Sprintf("- retention_users: %d", retentionUsers),
	}
	outboxTail := []string{
		fmt.Sprintf("- outbox_pending: %d", outbox["pending"]),
		fmt.Sprintf("- outbox_processing: %d", outbox["processing"]),
		fmt.Sprintf("- outbox_done: %d", outbox["done"]),
		fmt.Sprintf("- outbox_failed: %d", outbox["failed"]),
		fmt.Sprintf("- outbox_oldest_pending_age_sec: %d", ops.OldestPendingAgeSeconds),
		fmt.Sprintf("- outbox_pending_with_retries: %d", ops.PendingWithRetries),
		fmt.Sprintf("- outbox_sum_attempts_queued: %d", ops.SumAttemptsQueued),
		fmt.Sprintf("- wallet_balance_mismatches: %d", walletMismatches),
	}
	if len(counts) == 0 {
		return strings.Join(append(funnelTail, outboxTail...), "\n"), nil
	}
	lines := append([]string{}, funnelTail...)
	for k, v := range counts {
		lines = append(lines, fmt.Sprintf("- %s: %d", k, v))
	}
	lines = append(lines, outboxTail...)
	return strings.Join(lines, "\n"), nil
}
