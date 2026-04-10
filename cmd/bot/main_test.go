package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/bot"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/health"
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
	seenIDs   map[int]struct{}
	seenKeys  map[string]struct{}
}

func (f *fakeDedup) Seen(_ context.Context, updateID int) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.seenIDs != nil {
		if _, ok := f.seenIDs[updateID]; ok {
			return true, nil
		}
		f.seenIDs[updateID] = struct{}{}
		return false, nil
	}
	return f.duplicate, f.err
}

func (f *fakeDedup) SeenKey(_ context.Context, key string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.seenKeys != nil {
		if _, ok := f.seenKeys[key]; ok {
			return true, nil
		}
		f.seenKeys[key] = struct{}{}
		return false, nil
	}
	return f.duplicate, nil
}

type fakeLimiter struct {
	allowed bool
	err     error
}

func (f fakeLimiter) Allow(_ context.Context, _ int64, _ string) (bool, error) {
	return f.allowed, f.err
}

type spyLimiter struct {
	allowed bool
	err     error
	calls   int
}

func (s *spyLimiter) Allow(_ context.Context, _ int64, _ string) (bool, error) {
	s.calls++
	return s.allowed, s.err
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
	}, dispatchGuards{dedup: &fakeDedup{duplicate: true}})
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
	}, dispatchGuards{msgLimiter: fakeLimiter{allowed: false}})
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
	}, dispatchGuards{msgLimiter: fakeLimiter{err: errors.New("boom")}})
	if st.last == nil {
		t.Fatal("expected update to pass through when limiter fails")
	}
}

func TestDispatchTelegramUpdate_DropsGlobalRateLimitedMessage(t *testing.T) {
	st := &stubTelegram{}
	h := bot.Handlers{Bot: st, Logger: logging.NewWithWriter(&bytes.Buffer{})}
	dispatchTelegramUpdate(context.Background(), logging.NewWithWriter(&bytes.Buffer{}), &h, tgbotapi.Update{
		UpdateID: 780,
		Message:  &tgbotapi.Message{From: &tgbotapi.User{ID: 10}, Chat: &tgbotapi.Chat{ID: 10}, Text: "x"},
	}, dispatchGuards{
		globalLimiter: fakeLimiter{allowed: false},
		msgLimiter:    fakeLimiter{allowed: true},
	})
	if st.last != nil {
		t.Fatalf("expected globally rate-limited message to be dropped, got %T", st.last)
	}
}

func TestDispatchTelegramUpdate_DropsBlocklistedUserBeforeLimiters(t *testing.T) {
	st := &stubTelegram{}
	h := bot.Handlers{Bot: st, Logger: logging.NewWithWriter(&bytes.Buffer{})}
	global := &spyLimiter{allowed: true}
	msg := &spyLimiter{allowed: true}
	dispatchTelegramUpdate(context.Background(), logging.NewWithWriter(&bytes.Buffer{}), &h, tgbotapi.Update{
		UpdateID: 781,
		Message:  &tgbotapi.Message{From: &tgbotapi.User{ID: 999}, Chat: &tgbotapi.Chat{ID: 999}, Text: "x"},
	}, dispatchGuards{
		globalLimiter: global,
		msgLimiter:    msg,
		blocklist:     map[int64]struct{}{999: {}},
	})
	if st.last != nil {
		t.Fatalf("expected blocklisted message to be dropped, got %T", st.last)
	}
	if global.calls != 0 || msg.calls != 0 {
		t.Fatalf("expected limiters not called for blocklisted user, global=%d msg=%d", global.calls, msg.calls)
	}
}

func TestDispatchTelegramUpdate_DropsDuplicateCallbackID(t *testing.T) {
	st := &stubTelegram{}
	h := bot.Handlers{Bot: st, Logger: logging.NewWithWriter(&bytes.Buffer{})}
	dedup := &fakeDedup{seenIDs: map[int]struct{}{}, seenKeys: map[string]struct{}{}}
	logger := logging.NewWithWriter(&bytes.Buffer{})

	update := tgbotapi.Update{
		UpdateID: 9001,
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "cb-1",
			From: &tgbotapi.User{ID: 1},
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 2},
			},
			Data: "cmd:ping",
		},
	}
	dispatchTelegramUpdate(context.Background(), logger, &h, update, dispatchGuards{dedup: dedup})
	if st.last == nil {
		t.Fatal("expected first callback update to pass")
	}

	st.last = nil
	update.UpdateID = 9002
	dispatchTelegramUpdate(context.Background(), logger, &h, update, dispatchGuards{dedup: dedup})
	if st.last != nil {
		t.Fatalf("expected duplicate callback id to be dropped, got %T", st.last)
	}
}

func TestDispatchTelegramUpdate_DropsDuplicateChatMessageID(t *testing.T) {
	st := &stubTelegram{}
	h := bot.Handlers{Bot: st, Logger: logging.NewWithWriter(&bytes.Buffer{})}
	dedup := &fakeDedup{seenIDs: map[int]struct{}{}, seenKeys: map[string]struct{}{}}
	logger := logging.NewWithWriter(&bytes.Buffer{})

	update := tgbotapi.Update{
		UpdateID: 9101,
		Message: &tgbotapi.Message{
			MessageID: 77,
			From:      &tgbotapi.User{ID: 1},
			Chat:      &tgbotapi.Chat{ID: 101},
			Text:      "hello",
		},
	}
	dispatchTelegramUpdate(context.Background(), logger, &h, update, dispatchGuards{dedup: dedup})
	if st.last == nil {
		t.Fatal("expected first message update to pass")
	}

	st.last = nil
	update.UpdateID = 9102
	dispatchTelegramUpdate(context.Background(), logger, &h, update, dispatchGuards{dedup: dedup})
	if st.last != nil {
		t.Fatalf("expected duplicate chat/message id to be dropped, got %T", st.last)
	}
}

func TestPostReliabilityAlert_DispatchesExpectedPayload(t *testing.T) {
	gotPayload := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected content-type: %s", ct)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotPayload <- payload
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	err := postReliabilityAlert(context.Background(), srv.URL, "dedup_check_error", "telegram update dedup failed", map[string]any{
		"update_id": float64(123),
		"dedup_key": "update:123",
	})
	if err != nil {
		t.Fatalf("postReliabilityAlert returned error: %v", err)
	}

	var payload map[string]any
	select {
	case payload = <-gotPayload:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for webhook payload")
	}
	if payload["event"] != "dedup_check_error" {
		t.Fatalf("unexpected event: %#v", payload["event"])
	}
	if payload["message"] != "telegram update dedup failed" {
		t.Fatalf("unexpected message: %#v", payload["message"])
	}
	contextObj, ok := payload["context"].(map[string]any)
	if !ok {
		t.Fatalf("expected context object, got %T", payload["context"])
	}
	if contextObj["dedup_key"] != "update:123" {
		t.Fatalf("unexpected dedup_key: %#v", contextObj["dedup_key"])
	}
}

func TestPostReliabilityAlert_ReturnsErrorOnNon2xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := postReliabilityAlert(context.Background(), srv.URL, "global_limiter_check_error", "telegram global rate limit check failed", map[string]any{
		"telegram_user_id": float64(77),
	})
	if err == nil {
		t.Fatal("expected non-2xx response to return error")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected two attempts on non-2xx, got %d", got)
	}
}

func TestPostReliabilityAlert_RetriesAndSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	err := postReliabilityAlert(context.Background(), srv.URL, "dedup_check_error", "telegram update dedup failed", map[string]any{
		"update_id": float64(42),
	})
	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected second attempt to be used, got calls=%d", got)
	}
}

func TestReliabilityAlertEmitter_ThrottlesSameEvent(t *testing.T) {
	var calls atomic.Int32
	received := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		evt, _ := payload["event"].(string)
		calls.Add(1)
		received <- evt
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	var nowMu sync.Mutex
	emitter := newReliabilityAlertEmitterWithClock(
		srv.URL,
		logging.NewWithWriter(&bytes.Buffer{}),
		func() time.Time {
			nowMu.Lock()
			defer nowMu.Unlock()
			return now
		},
		60*time.Second,
	)

	emitter("global_limiter_check_error", "msg", nil)
	emitter("global_limiter_check_error", "msg", nil) // should be throttled
	emitter("health_server_startup_failure", "msg", nil)

	deadline := time.After(2 * time.Second)
	for calls.Load() < 2 {
		select {
		case <-received:
		case <-deadline:
			t.Fatalf("timeout waiting for expected webhook calls, got=%d", calls.Load())
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected only two calls before throttle window expires, got %d", got)
	}

	nowMu.Lock()
	now = now.Add(61 * time.Second)
	nowMu.Unlock()
	emitter("global_limiter_check_error", "msg", nil)

	deadline = time.After(2 * time.Second)
	for calls.Load() < 3 {
		select {
		case <-received:
		case <-deadline:
			t.Fatalf("timeout waiting for post-window call, got=%d", calls.Load())
		}
	}
}

func TestRunReadinessMonitor_AlertsOnlyOnTransitions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ticks := make(chan time.Time, 8)
	states := []bool{
		true,  // initial baseline
		true,  // no transition
		false, // degraded
		false, // no transition
		true,  // recovered
	}
	var mu sync.Mutex
	check := func() (health.ReadinessSnapshot, bool) {
		mu.Lock()
		defer mu.Unlock()
		ready := true
		if len(states) > 0 {
			ready = states[0]
			states = states[1:]
		}
		status := "ready"
		if !ready {
			status = "not_ready"
		}
		return health.ReadinessSnapshot{
			Status: status,
			Checks: map[string]health.ReadinessCheck{
				"database": {OK: ready, Detail: "test"},
				"redis":    {OK: ready, Detail: "test"},
				"outbox":   {OK: true, Detail: "enabled"},
			},
		}, ready
	}

	type emission struct {
		event string
		msg   string
	}
	got := make(chan emission, 4)
	alert := func(event, message string, _ map[string]any) {
		got <- emission{event: event, msg: message}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		runReadinessMonitor(ctx, logging.NewWithWriter(&bytes.Buffer{}), ticks, check, alert)
	}()

	ticks <- time.Now()
	ticks <- time.Now()
	ticks <- time.Now()
	ticks <- time.Now()

	var events []emission
	deadline := time.After(2 * time.Second)
	for len(events) < 2 {
		select {
		case e := <-got:
			events = append(events, e)
		case <-deadline:
			t.Fatalf("timeout waiting for transition alerts, got=%d", len(events))
		}
	}
	if len(events) != 2 {
		t.Fatalf("expected exactly 2 transition alerts, got=%d", len(events))
	}
	if events[0].event != "readiness_degraded" {
		t.Fatalf("expected first transition to degraded, got=%q", events[0].event)
	}
	if events[1].event != "readiness_recovered" {
		t.Fatalf("expected second transition to recovered, got=%q", events[1].event)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("monitor did not stop on context cancellation")
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

func TestTelegramGlobalRateLimitPerMinute(t *testing.T) {
	t.Setenv("TELEGRAM_RATE_LIMIT_GLOBAL_PER_MIN", "11")
	if got := telegramGlobalRateLimitPerMinute(); got != 11 {
		t.Fatalf("unexpected global rate limit: %d", got)
	}
}

func TestTelegramBlocklistUserIDs(t *testing.T) {
	t.Setenv("TELEGRAM_BLOCKLIST_USER_IDS", " 1001, bad, 1002 ,,1001 ")
	ids := telegramBlocklistUserIDs()
	if _, ok := ids[1001]; !ok {
		t.Fatal("expected 1001 in blocklist")
	}
	if _, ok := ids[1002]; !ok {
		t.Fatal("expected 1002 in blocklist")
	}
	if len(ids) != 2 {
		t.Fatalf("expected exactly two unique IDs, got %d", len(ids))
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
