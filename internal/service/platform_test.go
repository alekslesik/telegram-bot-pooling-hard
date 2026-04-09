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

	report, err := svc.AdminAnalyticsReport(ctx, adminID)
	if err != nil {
		t.Fatalf("admin analytics report error: %v", err)
	}
	if !strings.Contains(report, "wallet_balance_mismatches: 1") {
		t.Fatalf("expected wallet mismatch metric in report, got:\n%s", report)
	}
}
