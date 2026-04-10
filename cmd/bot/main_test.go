package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/bot"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/logging"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
)

func TestFormatBuildDate_RFC3339(t *testing.T) {
	raw := time.Date(2024, 6, 10, 15, 30, 0, 0, time.UTC).Format(time.RFC3339)
	got := formatBuildDate(raw)
	// Europe/Moscow is UTC+3 in June (no DST).
	if want := "10/06/2024 18:30:00"; got != want {
		t.Fatalf("formatBuildDate(%q) = %q, want %q", raw, got, want)
	}
}

func TestFormatBuildDate_RFC3339Nano(t *testing.T) {
	raw := time.Date(2024, 1, 2, 3, 4, 5, 123456789, time.UTC).Format(time.RFC3339Nano)
	got := formatBuildDate(raw)
	if want := "02/01/2024 06:04:05"; got != want {
		t.Fatalf("formatBuildDate(nano) = %q, want %q", got, want)
	}
}

func TestFormatBuildDate_nonDatePassthrough(t *testing.T) {
	const s = "not-a-date"
	if formatBuildDate(s) != s {
		t.Fatalf("expected passthrough %q, got %q", s, formatBuildDate(s))
	}
}

func TestFormatBuildDate_loadLocationFails(t *testing.T) {
	orig := loadEuropeMoscow
	t.Cleanup(func() { loadEuropeMoscow = orig })
	loadEuropeMoscow = func() (*time.Location, error) {
		return nil, errors.New("no tz")
	}
	raw := time.Date(2024, 6, 10, 15, 30, 0, 0, time.UTC).Format(time.RFC3339)
	if got, want := formatBuildDate(raw), "10/06/2024 15:30:00"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

type stubTelegram struct {
	last tgbotapi.Chattable
	err  error
}

type fakeDedup struct {
	duplicate bool
	err       error
}

func (f fakeDedup) Seen(_ context.Context, _ int) (bool, error) {
	return f.duplicate, f.err
}

type fakeLimiter struct {
	allowed bool
	err     error
}

func (f fakeLimiter) Allow(_ context.Context, _ int64, _ string) (bool, error) {
	return f.allowed, f.err
}

const errUnexpectedFmt = "unexpected error: %v"

func (s *stubTelegram) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	s.last = c
	return tgbotapi.Message{}, nil
}

func (s *stubTelegram) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	s.last = c
	if s.err != nil {
		return nil, s.err
	}
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func TestApplyTelegramUpdate_message(t *testing.T) {
	st := &stubTelegram{}
	h := bot.Handlers{
		Bot:    st,
		Logger: logging.NewWithWriter(&bytes.Buffer{}),
	}
	applyTelegramUpdate(&h, tgbotapi.Update{
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1}, Text: "hi"},
	})
	if _, ok := st.last.(tgbotapi.MessageConfig); !ok {
		t.Fatalf("expected send, got %T", st.last)
	}
}

func TestApplyTelegramUpdate_callback(t *testing.T) {
	st := &stubTelegram{}
	h := bot.Handlers{
		Bot:    st,
		Logger: logging.NewWithWriter(&bytes.Buffer{}),
	}
	applyTelegramUpdate(&h, tgbotapi.Update{
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:      "x",
			Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 2}},
			Data:    "cmd:ping",
		},
	})
	if st.last == nil {
		t.Fatal("expected Request or Send")
	}
}

func TestApplyTelegramUpdate_empty(t *testing.T) {
	st := &stubTelegram{}
	h := bot.Handlers{Bot: st, Logger: logging.NewWithWriter(&bytes.Buffer{})}
	applyTelegramUpdate(&h, tgbotapi.Update{})
	if st.last != nil {
		t.Fatalf("expected no traffic, got %T", st.last)
	}
}

func TestDispatchTelegramUpdate_DropsDuplicate(t *testing.T) {
	st := &stubTelegram{}
	h := bot.Handlers{Bot: st, Logger: logging.NewWithWriter(&bytes.Buffer{})}
	dispatchTelegramUpdate(context.Background(), logging.NewWithWriter(&bytes.Buffer{}), &h, tgbotapi.Update{
		UpdateID: 777,
		Message:  &tgbotapi.Message{From: &tgbotapi.User{ID: 1}, Chat: &tgbotapi.Chat{ID: 1}, Text: "x"},
	}, fakeDedup{duplicate: true}, nil, nil)
	if st.last != nil {
		t.Fatalf("expected duplicate update to be dropped, got %T", st.last)
	}
}

func TestDispatchTelegramUpdate_DropsRateLimitedMessage(t *testing.T) {
	st := &stubTelegram{}
	h := bot.Handlers{Bot: st, Logger: logging.NewWithWriter(&bytes.Buffer{})}
	dispatchTelegramUpdate(context.Background(), logging.NewWithWriter(&bytes.Buffer{}), &h, tgbotapi.Update{
		UpdateID: 778,
		Message:  &tgbotapi.Message{From: &tgbotapi.User{ID: 1}, Chat: &tgbotapi.Chat{ID: 1}, Text: "x"},
	}, nil, fakeLimiter{allowed: false}, nil)
	if st.last != nil {
		t.Fatalf("expected rate-limited message to be dropped, got %T", st.last)
	}
}

func TestDispatchTelegramUpdate_AllowsWhenLimiterErrors(t *testing.T) {
	st := &stubTelegram{}
	h := bot.Handlers{Bot: st, Logger: logging.NewWithWriter(&bytes.Buffer{})}
	dispatchTelegramUpdate(context.Background(), logging.NewWithWriter(&bytes.Buffer{}), &h, tgbotapi.Update{
		UpdateID: 779,
		Message:  &tgbotapi.Message{From: &tgbotapi.User{ID: 1}, Chat: &tgbotapi.Chat{ID: 1}, Text: "x"},
	}, nil, fakeLimiter{err: errors.New("boom")}, nil)
	if st.last == nil {
		t.Fatal("expected update to pass through when limiter fails")
	}
}

func TestLogAuthorized_withExpectedUsername(t *testing.T) {
	var buf bytes.Buffer
	logAuthorized(logging.NewWithWriter(&buf), "want", "got")
	if !bytes.Contains(buf.Bytes(), []byte("expected_username")) {
		t.Fatalf("log: %s", buf.String())
	}
}

func TestLogAuthorized_withoutExpectedUsername(t *testing.T) {
	var buf bytes.Buffer
	logAuthorized(logging.NewWithWriter(&buf), "", "only")
	if bytes.Contains(buf.Bytes(), []byte("expected_username")) {
		t.Fatalf("unexpected field: %s", buf.String())
	}
}

func TestTokenFromEnv(t *testing.T) {
	t.Setenv("TOKEN", "  abc  ")
	if got := tokenFromEnv(); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestLongPollTimeoutSeconds(t *testing.T) {
	if longPollTimeoutSeconds() != 60 {
		t.Fatal("unexpected long poll timeout")
	}
}

func TestTelegramRateLimitConfigDefaults(t *testing.T) {
	t.Setenv("TELEGRAM_RATE_LIMIT_MSG_PER_MIN", "")
	t.Setenv("TELEGRAM_RATE_LIMIT_CALLBACK_PER_MIN", "")
	msg, callback := telegramRateLimitConfig()
	if msg != 0 || callback != 0 {
		t.Fatalf("expected disabled defaults, got msg=%d callback=%d", msg, callback)
	}
}

func TestTelegramRateLimitConfigValues(t *testing.T) {
	t.Setenv("TELEGRAM_RATE_LIMIT_MSG_PER_MIN", "5")
	t.Setenv("TELEGRAM_RATE_LIMIT_CALLBACK_PER_MIN", "7")
	msg, callback := telegramRateLimitConfig()
	if msg != 5 || callback != 7 {
		t.Fatalf("unexpected config: msg=%d callback=%d", msg, callback)
	}
}

func TestClearBotCommands(t *testing.T) {
	st := &stubTelegram{}
	var buf bytes.Buffer
	clearBotCommands(st, logging.NewWithWriter(&buf))
	if _, ok := st.last.(tgbotapi.DeleteMyCommandsConfig); !ok {
		t.Fatalf("expected DeleteMyCommandsConfig, got %T", st.last)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no error log, got %s", buf.String())
	}
}

func TestClearBotCommands_ErrorLogged(t *testing.T) {
	st := &stubTelegram{err: errors.New("boom")}
	var buf bytes.Buffer
	clearBotCommands(st, logging.NewWithWriter(&buf))
	if !bytes.Contains(buf.Bytes(), []byte("failed to clear bot commands")) {
		t.Fatalf("unexpected log output: %s", buf.String())
	}
}

func TestLoadClinicRefundPolicyFromEnvDefaults(t *testing.T) {
	t.Setenv("CLINIC_REFUND_PARTIAL_WINDOW", "")
	t.Setenv("CLINIC_REFUND_PARTIAL_PERCENT", "")

	p, err := loadClinicRefundPolicyFromEnv()
	if err != nil {
		t.Fatalf(errUnexpectedFmt, err)
	}
	if p.PartialWindow != 24*time.Hour {
		t.Fatalf("unexpected default window: %s", p.PartialWindow)
	}
	if p.PartialPercent != 50 {
		t.Fatalf("unexpected default percent: %d", p.PartialPercent)
	}
}

func TestLoadClinicRefundPolicyFromEnvOverrides(t *testing.T) {
	t.Setenv("CLINIC_REFUND_PARTIAL_WINDOW", "6h")
	t.Setenv("CLINIC_REFUND_PARTIAL_PERCENT", "25")

	p, err := loadClinicRefundPolicyFromEnv()
	if err != nil {
		t.Fatalf(errUnexpectedFmt, err)
	}
	if p.PartialWindow != 6*time.Hour {
		t.Fatalf("unexpected window: %s", p.PartialWindow)
	}
	if p.PartialPercent != 25 {
		t.Fatalf("unexpected percent: %d", p.PartialPercent)
	}
}

func TestLoadClinicRefundPolicyFromEnvInvalid(t *testing.T) {
	t.Setenv("CLINIC_REFUND_PARTIAL_WINDOW", "bad")
	t.Setenv("CLINIC_REFUND_PARTIAL_PERCENT", "101")

	if _, err := loadClinicRefundPolicyFromEnv(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestBuildBookingRepositoryAppliesConfiguredRefundPolicy(t *testing.T) {
	t.Setenv("DB_DSN", "")
	t.Setenv("DB_PASSWORD_FILE", "")
	t.Setenv("CLINIC_REFUND_PARTIAL_WINDOW", "1000h")
	t.Setenv("CLINIC_REFUND_PARTIAL_PERCENT", "10")

	repo, db, err := buildBookingRepository(logging.NewWithWriter(&bytes.Buffer{}))
	if err != nil {
		t.Fatalf(errUnexpectedFmt, err)
	}
	if db != nil {
		t.Fatal("expected nil db for in-memory mode")
	}

	mem, ok := repo.(*repository.MemoryRepository)
	if !ok {
		t.Fatalf("expected *MemoryRepository, got %T", repo)
	}

	ctx := context.Background()
	const userID int64 = 991
	if _, err := mem.EnsureUserProfile(ctx, userID); err != nil {
		t.Fatalf("ensure profile: %v", err)
	}

	paid, err := mem.ConfirmPaidClinicBooking(ctx, userID, 100, 1, 1, 1, "op-policy-env-991")
	if err != nil {
		t.Fatalf("confirm paid: %v", err)
	}
	res, err := mem.CancelClinicBooking(ctx, userID, paid.BookingID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !res.RefundApplied || res.RefundedCents != 10 {
		t.Fatalf("expected 10-cent partial refund from env policy, got applied=%v refunded=%d", res.RefundApplied, res.RefundedCents)
	}
}
