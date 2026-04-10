package service

import (
	"context"
	"strings"
	"testing"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
)

func TestBookingServiceAdminAnalyticsReportIncludesWalletMismatches(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewBookingService(repo, nil)
	ctx := context.Background()

	const (
		adminID int64 = 8001
		userID  int64 = 8002
	)

	repo.SetAdminRole(adminID, repository.AdminRoleAdmin)
	if _, err := repo.EnsureUserProfile(ctx, userID); err != nil {
		t.Fatalf("ensure user profile: %v", err)
	}

	report, err := svc.AdminAnalyticsReport(ctx, adminID, 7, nil)
	if err != nil {
		t.Fatalf("admin analytics report error: %v", err)
	}
	if !strings.Contains(report, "wallet_balance_mismatches: 1") {
		t.Fatalf("expected wallet mismatch metric in report, got:\n%s", report)
	}
}

func TestBookingServiceAdminAnalyticsReportIncludesPeriodSegmentAndAggregates(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := NewBookingService(repo, nil)
	ctx := context.Background()

	const (
		adminID int64 = 8101
		userID  int64 = 8102
	)
	repo.SetAdminRole(adminID, repository.AdminRoleAdmin)
	if _, err := repo.EnsureUserProfile(ctx, userID); err != nil {
		t.Fatalf("ensure user profile: %v", err)
	}
	if _, err := repo.ConfirmPaidClinicBooking(ctx, userID, 100, 1, 1, 1, "op-platform-report-1"); err != nil {
		t.Fatalf("confirm booking: %v", err)
	}
	uidp := userID
	if err := svc.LogAnalytics(ctx, &uidp, "funnel_book_specialty_selected", `{"specialty_id":1}`); err != nil {
		t.Fatalf("log analytics funnel #1: %v", err)
	}
	if err := svc.LogAnalytics(ctx, &uidp, "funnel_book_specialty_selected", `{"specialty_id":1}`); err != nil {
		t.Fatalf("log analytics funnel #2: %v", err)
	}
	if err := svc.LogAnalytics(ctx, &uidp, "cmd_start", `{}`); err != nil {
		t.Fatalf("log analytics cmd_start: %v", err)
	}
	if err := svc.LogAnalytics(ctx, &uidp, "booking_confirmed", `{"spec_id":1,"doc_id":1,"slot_id":1}`); err != nil {
		t.Fatalf("log analytics booking_confirmed: %v", err)
	}

	specID := int64(1)
	report, err := svc.AdminAnalyticsReport(ctx, adminID, 30, &specID)
	if err != nil {
		t.Fatalf("admin analytics report error: %v", err)
	}
	for _, needle := range []string{
		"period: last 30 days",
		"segment: specialty_id=1",
		"funnel_conversion_approx_confirmed_per_specialty_selected:",
		"bookings_confirmed_total:",
		"cancellations:",
		"no_show_proxy:",
		"referral_rewards_granted:",
		"retention_users:",
		"outbox_pending:",
		"wallet_balance_mismatches:",
	} {
		if !strings.Contains(report, needle) {
			t.Fatalf("expected report to contain %q, got:\n%s", needle, report)
		}
	}
	if !strings.Contains(report, "retention_users: 1") {
		t.Fatalf("expected non-zero retention metric in report, got:\n%s", report)
	}
}
