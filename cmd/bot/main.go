package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/bot"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/cache"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/dbconfig"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/health"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/logging"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/service"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/telegram"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/telegramguard"
	_ "github.com/lib/pq"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// loadEuropeMoscow is swappable in tests to cover the LoadLocation error path.
var loadEuropeMoscow = func() (*time.Location, error) {
	return time.LoadLocation("Europe/Moscow")
}

// formatBuildDate turns an RFC3339 / RFC3339Nano build timestamp into log display format (Europe/Moscow).
func formatBuildDate(raw string) string {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		if loc, locErr := loadEuropeMoscow(); locErr == nil {
			t = t.In(loc)
		}
		return t.Format("02/01/2006 15:04:05")
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		if loc, locErr := loadEuropeMoscow(); locErr == nil {
			t = t.In(loc)
		}
		return t.Format("02/01/2006 15:04:05")
	}
	return raw
}

func applyTelegramUpdate(h *bot.Handlers, u tgbotapi.Update) {
	if u.CallbackQuery != nil {
		h.HandleCallback(u.CallbackQuery)
		return
	}
	if u.Message == nil {
		return
	}
	h.HandleMessage(u.Message)
}

func logAuthorized(logger slogLogger, username, botUsername string) {
	if username != "" {
		logger.Info("authorized",
			"username", botUsername,
			"expected_username", username,
		)
	} else {
		logger.Info("authorized",
			"username", botUsername,
		)
	}
}

// slogLogger is the subset of *slog.Logger used by main (tests pass a concrete *slog.Logger).
type slogLogger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
}

type updateDeduplicator interface {
	Seen(ctx context.Context, updateID int) (bool, error)
	SeenKey(ctx context.Context, key string) (bool, error)
}

type updateRateLimiter interface {
	Allow(ctx context.Context, telegramUserID int64, kind string) (bool, error)
}

type commandRegistrar interface {
	Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

type reliabilityAlertEmitter func(event, message string, contextFields map[string]any)

const readinessMonitorInterval = 30 * time.Second

type dispatchGuards struct {
	dedup           updateDeduplicator
	globalLimiter   updateRateLimiter
	msgLimiter      updateRateLimiter
	callbackLimiter updateRateLimiter
	blocklist       map[int64]struct{}
	alert           reliabilityAlertEmitter
}

func clearBotCommands(reg commandRegistrar, logger slogLogger) {
	if _, err := reg.Request(tgbotapi.NewDeleteMyCommands()); err != nil {
		logger.Error("failed to clear bot commands", "err", err)
	}
}

func tokenFromEnv() string {
	return strings.TrimSpace(os.Getenv("TOKEN"))
}

func longPollTimeoutSeconds() int {
	return 60
}

func main() {
	logger := logging.NewFromEnv()
	reliabilityAlerts := newReliabilityAlertEmitter(strings.TrimSpace(os.Getenv("RELIABILITY_ALERT_WEBHOOK")), logger)

	buildDate := formatBuildDate(BuildDate)

	logger.Info("starting",
		"version", Version,
		"commit", Commit,
		"build_date", buildDate,
	)

	token := tokenFromEnv()
	if token == "" {
		log.Fatal("env var TOKEN is not set (see .env)")
	}

	username := os.Getenv("USERNAME")
	tg, err := initTelegramClient(token)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	logAuthorized(logger, username, tg.Self.UserName)
	clearBotCommands(tg, logger)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = longPollTimeoutSeconds()

	updates := tg.GetUpdatesChan(u)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	bookingRepo, dbConn, err := buildBookingRepository(logger)
	if err != nil {
		log.Fatalf("failed to init booking repository: %v", err)
	}

	redisCache, globalLimiter, msgLimiter, callbackLimiter, updateDedup, blocklist, err := initTelegramGuards(logger)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	if redisCache != nil {
		defer func() { _ = redisCache.Close() }()
	}

	var specCache service.SpecialtyPageCache = redisCache
	bookingService := service.NewBookingService(bookingRepo, specCache)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	h := bot.Handlers{
		Bot:         tg,
		Logger:      logger,
		Booking:     bookingService,
		BotUsername: tg.Self.UserName,
	}

	startOutboxWorker(workerCtx, logger, tg, bookingRepo)
	healthSrv := startHealthServer(logger, dbConn, redisCache, reliabilityAlerts)
	startReadinessMonitor(workerCtx, logger, dbConn, redisCache, outboxWorkerEnabled(), reliabilityAlerts, readinessMonitorInterval)

	logger.Info("bot started with long polling, press Ctrl+C to stop")

	guards := dispatchGuards{
		dedup:           updateDedup,
		globalLimiter:   globalLimiter,
		msgLimiter:      msgLimiter,
		callbackLimiter: callbackLimiter,
		blocklist:       blocklist,
		alert:           reliabilityAlerts,
	}
	for {
		select {
		case update := <-updates:
			dispatchTelegramUpdate(context.Background(), logger, &h, update, guards)

		case sig := <-stop:
			logger.Info("received signal, shutting down", "signal", sig.String())
			workerCancel()
			if healthSrv != nil {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = healthSrv.Shutdown(shutdownCtx)
				cancel()
			}
			return
		}
	}
}

func startReadinessMonitor(
	ctx context.Context,
	logger slogLogger,
	dbConn *sql.DB,
	redisCache *cache.Redis,
	outboxEnabled bool,
	alert reliabilityAlertEmitter,
	interval time.Duration,
) {
	if alert == nil || interval <= 0 {
		return
	}
	check := func() (health.ReadinessSnapshot, bool) {
		return health.EvaluateReadiness(dbConn, redisCache, outboxEnabled)
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		runReadinessMonitor(ctx, logger, ticker.C, check, alert)
	}()
}

func runReadinessMonitor(
	ctx context.Context,
	logger slogLogger,
	ticks <-chan time.Time,
	check func() (health.ReadinessSnapshot, bool),
	alert reliabilityAlertEmitter,
) {
	if alert == nil || check == nil {
		return
	}
	_, prevReady := check()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			snapshot, ready := check()
			if ready == prevReady {
				continue
			}
			prevReady = ready
			event := "readiness_degraded"
			msg := "readiness transitioned to not_ready"
			if ready {
				event = "readiness_recovered"
				msg = "readiness transitioned to ready"
			}
			alert(event, msg, map[string]any{
				"status": snapshot.Status,
				"checks": snapshot.Checks,
			})
			logger.Warn("readiness state transitioned", "event", event, "status", snapshot.Status)
		}
	}
}

func initTelegramClient(token string) (*tgbotapi.BotAPI, error) {
	return telegram.New(token)
}

func dispatchTelegramUpdate(
	ctx context.Context,
	logger slogLogger,
	h *bot.Handlers,
	u tgbotapi.Update,
	guards dispatchGuards,
) {
	if isDuplicateUpdate(ctx, logger, u, guards) {
		return
	}

	userID, kind, ok := telegramUpdateIdentity(u)
	if ok {
		if _, blocked := guards.blocklist[userID]; blocked {
			logger.Warn(
				"telegram update blocked by operator blocklist",
				"telegram_user_id", userID,
				"kind", kind,
				"update_id", u.UpdateID,
			)
			return
		}
		if !allowByGlobalLimit(ctx, logger, guards, userID, kind, u.UpdateID) {
			return
		}
		if !allowByPerUserLimit(ctx, logger, guards, userID, kind) {
			return
		}
	}
	applyTelegramUpdate(h, u)
}

func isDuplicateUpdate(ctx context.Context, logger slogLogger, u tgbotapi.Update, guards dispatchGuards) bool {
	for _, dedupKey := range telegramUpdateDedupKeys(u) {
		duplicate, err := dedupSeen(ctx, guards.dedup, dedupKey)
		if err != nil {
			logger.Error("telegram update dedup failed", "err", err, "dedup_key", dedupKey)
			if guards.alert != nil {
				guards.alert("dedup_check_error", "telegram update dedup failed", map[string]any{
					"error":     err.Error(),
					"dedup_key": dedupKey,
					"update_id": u.UpdateID,
				})
			}
			continue
		}
		if duplicate {
			return true
		}
	}
	return false
}

func allowByGlobalLimit(ctx context.Context, logger slogLogger, guards dispatchGuards, userID int64, kind string, updateID int) bool {
	if guards.globalLimiter == nil {
		return true
	}
	allowed, err := guards.globalLimiter.Allow(ctx, 0, "global")
	if err != nil {
		logger.Error("telegram global rate limit check failed", "err", err, "telegram_user_id", userID, "kind", kind)
		if guards.alert != nil {
			guards.alert("global_limiter_check_error", "telegram global rate limit check failed", map[string]any{
				"error":            err.Error(),
				"telegram_user_id": userID,
				"kind":             kind,
				"update_id":        updateID,
			})
		}
		return true
	}
	if !allowed {
		logger.Warn("telegram update globally rate limited", "telegram_user_id", userID, "kind", kind)
		return false
	}
	return true
}

func allowByPerUserLimit(ctx context.Context, logger slogLogger, guards dispatchGuards, userID int64, kind string) bool {
	limiter := guards.msgLimiter
	if kind == "callback" {
		limiter = guards.callbackLimiter
	}
	if limiter == nil {
		return true
	}
	allowed, err := limiter.Allow(ctx, userID, kind)
	if err != nil {
		logger.Error("telegram rate limit check failed", "err", err, "telegram_user_id", userID, "kind", kind)
		return true
	}
	if !allowed {
		logger.Warn("telegram update rate limited", "telegram_user_id", userID, "kind", kind)
		return false
	}
	return true
}

func dedupSeen(ctx context.Context, dedup updateDeduplicator, dedupKey string) (bool, error) {
	if dedup == nil || dedupKey == "" {
		return false, nil
	}
	if strings.HasPrefix(dedupKey, "update:") {
		updateID, err := strconv.Atoi(strings.TrimPrefix(dedupKey, "update:"))
		if err != nil {
			return false, nil
		}
		return dedup.Seen(ctx, updateID)
	}
	return dedup.SeenKey(ctx, dedupKey)
}

func telegramUpdateDedupKeys(u tgbotapi.Update) []string {
	keys := make([]string, 0, 3)
	if u.UpdateID != 0 {
		keys = append(keys, fmt.Sprintf("update:%d", u.UpdateID))
	}
	if u.CallbackQuery != nil && strings.TrimSpace(u.CallbackQuery.ID) != "" {
		keys = append(keys, "callback:"+u.CallbackQuery.ID)
	}
	if u.Message != nil && u.Message.Chat != nil && u.Message.MessageID != 0 {
		keys = append(keys, fmt.Sprintf("message:%d:%d", u.Message.Chat.ID, u.Message.MessageID))
	}
	return keys
}

func telegramUpdateIdentity(u tgbotapi.Update) (int64, string, bool) {
	if u.CallbackQuery != nil && u.CallbackQuery.From != nil {
		return u.CallbackQuery.From.ID, "callback", true
	}
	if u.Message != nil && u.Message.From != nil {
		return u.Message.From.ID, "message", true
	}
	return 0, "", false
}

func telegramRateLimitConfig() (int64, int64) {
	return parseInt64Env("TELEGRAM_RATE_LIMIT_MSG_PER_MIN", 0), parseInt64Env("TELEGRAM_RATE_LIMIT_CALLBACK_PER_MIN", 0)
}

func telegramGlobalRateLimitPerMinute() int64 {
	return parseInt64Env("TELEGRAM_RATE_LIMIT_GLOBAL_PER_MIN", 0)
}

func telegramBlocklistUserIDs() map[int64]struct{} {
	raw := strings.TrimSpace(os.Getenv("TELEGRAM_BLOCKLIST_USER_IDS"))
	if raw == "" {
		return map[int64]struct{}{}
	}
	ids := make(map[int64]struct{})
	for _, p := range strings.Split(raw, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil {
			continue
		}
		ids[id] = struct{}{}
	}
	return ids
}

func parseInt64Env(key string, def int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 0 {
		return def
	}
	return v
}

func buildUpdateLimiters(logger slogLogger, redisCache *cache.Redis, msgPerMinute int64, callbackPerMinute int64) (*telegramguard.Limiter, *telegramguard.Limiter) {
	if msgPerMinute <= 0 && callbackPerMinute <= 0 {
		logger.Info("telegram rate limits disabled", "reason", "configured as disabled")
		return nil, nil
	}
	if redisCache == nil {
		logger.Info("telegram rate limits disabled", "reason", "redis unavailable")
		return nil, nil
	}
	msg := telegramguard.NewLimiter(redisCache, msgPerMinute, 60, "tg:rl")
	callback := telegramguard.NewLimiter(redisCache, callbackPerMinute, 60, "tg:rl")
	return msg, callback
}

func buildUpdateDeduplicator(logger slogLogger, redisCache *cache.Redis) *telegramguard.Deduplicator {
	if redisCache == nil {
		logger.Info("telegram update dedup disabled", "reason", "redis unavailable")
		return nil
	}
	return telegramguard.NewDeduplicator(redisCache, 24*time.Hour, "tg:upd")
}

func initTelegramGuards(logger slogLogger) (*cache.Redis, *telegramguard.Limiter, *telegramguard.Limiter, *telegramguard.Limiter, *telegramguard.Deduplicator, map[int64]struct{}, error) {
	redisCache, err := cache.NewRedisFromEnv()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	if redisCache != nil {
		logger.Info("redis cache enabled for specialty pages")
	}
	globalLimit := telegramGlobalRateLimitPerMinute()
	msgLimit, callbackLimit := telegramRateLimitConfig()
	globalLimiter := telegramguard.NewLimiter(redisCache, globalLimit, 60, "tg:rl:global")
	msgLimiter, callbackLimiter := buildUpdateLimiters(logger, redisCache, msgLimit, callbackLimit)
	updateDedup := buildUpdateDeduplicator(logger, redisCache)
	blocklist := telegramBlocklistUserIDs()
	return redisCache, globalLimiter, msgLimiter, callbackLimiter, updateDedup, blocklist, nil
}

func startOutboxWorker(ctx context.Context, logger slogLogger, tg *tgbotapi.BotAPI, bookingRepo repository.BookingRepository) {
	if !outboxWorkerEnabled() {
		return
	}
	reminderNotifier := func(ctx context.Context, userID int64, text string) error {
		msg := tgbotapi.NewMessage(userID, text)
		_, err := tg.Send(msg)
		return err
	}
	var workerOpts []func(*service.OutboxWorker)
	if u := strings.TrimSpace(os.Getenv("OUTBOX_DEAD_LETTER_WEBHOOK")); u != "" {
		logger.Info("outbox dead-letter webhook enabled")
		workerOpts = append(workerOpts, service.WithDeadLetterHook(deadLetterWebhookHook(u)))
	}
	outboxWorker := service.NewOutboxWorker(bookingRepo, service.NewBookingOutboxHandler(bookingRepo, reminderNotifier), 20, 30*time.Second, workerOpts...)
	go outboxWorker.Run(ctx, 2*time.Second)
	logger.Info("outbox worker enabled")
}

func startHealthServer(logger slogLogger, dbConn *sql.DB, redisCache *cache.Redis, alert reliabilityAlertEmitter) *health.Server {
	addr := healthAddrFromEnv()
	if addr == "" {
		return nil
	}
	healthSrv := health.NewServer(addr, dbConn, redisCache, outboxWorkerEnabled(), Version, Commit)
	go func() {
		if err := healthSrv.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("health server failed", "err", err)
			if alert != nil {
				alert("health_server_startup_failure", "health server failed", map[string]any{
					"error": err.Error(),
					"addr":  addr,
				})
			}
		}
	}()
	logger.Info("health endpoints enabled", "addr", addr)
	return healthSrv
}

func postReliabilityAlert(ctx context.Context, webhookURL, event, message string, contextFields map[string]any) error {
	payload := map[string]any{
		"event":   event,
		"message": message,
	}
	if len(contextFields) > 0 {
		payload["context"] = contextFields
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Second}
	const attempts = 2
	const backoff = 150 * time.Millisecond
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
		if reqErr != nil {
			return reqErr
		}
		req.Header.Set("Content-Type", "application/json")
		resp, doErr := client.Do(req)
		if doErr == nil {
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
				return nil
			}
			doErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
		lastErr = doErr
		if attempt == attempts {
			break
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func newReliabilityAlertEmitter(webhookURL string, logger slogLogger) reliabilityAlertEmitter {
	return newReliabilityAlertEmitterWithClock(webhookURL, logger, time.Now, 60*time.Second)
}

func newReliabilityAlertEmitterWithClock(webhookURL string, logger slogLogger, now func() time.Time, throttleWindow time.Duration) reliabilityAlertEmitter {
	if strings.TrimSpace(webhookURL) == "" {
		return nil
	}
	var mu sync.Mutex
	lastSentByEvent := make(map[string]time.Time)
	return func(event, message string, contextFields map[string]any) {
		eventKey := strings.TrimSpace(event)
		if eventKey == "" {
			eventKey = "_unknown"
		}
		mu.Lock()
		lastSentAt, exists := lastSentByEvent[eventKey]
		if exists && now().Sub(lastSentAt) < throttleWindow {
			mu.Unlock()
			return
		}
		lastSentByEvent[eventKey] = now()
		mu.Unlock()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := postReliabilityAlert(ctx, webhookURL, event, message, contextFields); err != nil {
				logger.Warn("reliability alert delivery failed", "event", event, "err", err)
			}
		}()
	}
}

func outboxWorkerEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("OUTBOX_WORKER_ENABLED")))
	return raw == "" || raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

// deadLetterWebhookHook POSTs JSON to OUTBOX_DEAD_LETTER_WEBHOOK after an event is marked failed (RFC §2 alerting).
func deadLetterWebhookHook(url string) func(context.Context, repository.OutboxEvent, error) {
	return func(ctx context.Context, ev repository.OutboxEvent, handlerErr error) {
		payload := map[string]any{
			"event_id":   ev.ID,
			"event_type": ev.EventType,
			"dedupe_key": ev.DedupeKey,
			"last_error": handlerErr.Error(),
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		_ = err
	}
}

func buildBookingRepository(logger slogLogger) (repository.BookingRepository, *sql.DB, error) {
	refundPolicy, err := loadClinicRefundPolicyFromEnv()
	if err != nil {
		return nil, nil, err
	}
	dsn, err := dbconfig.ResolveDSN()
	if err != nil {
		return nil, nil, err
	}
	if dsn == "" {
		logger.Info("DB_DSN / DB_PASSWORD_FILE not set, using in-memory booking repository")
		mem := repository.NewMemoryRepository()
		if err := mem.SetClinicBookingRefundPolicy(refundPolicy); err != nil {
			return nil, nil, err
		}
		return mem, nil, nil
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, nil, err
	}
	logger.Info("postgres booking repository enabled")
	pg := repository.NewPostgresRepository(db)
	if err := pg.SetClinicBookingRefundPolicy(refundPolicy); err != nil {
		return nil, nil, err
	}
	return pg, db, nil
}

func healthAddrFromEnv() string {
	return strings.TrimSpace(os.Getenv("HTTP_HEALTH_ADDR"))
}

func loadClinicRefundPolicyFromEnv() (repository.ClinicBookingRefundPolicy, error) {
	policy := repository.DefaultClinicBookingRefundPolicy()

	if raw := strings.TrimSpace(os.Getenv("CLINIC_REFUND_PARTIAL_WINDOW")); raw != "" {
		v, err := time.ParseDuration(raw)
		if err != nil {
			return repository.ClinicBookingRefundPolicy{}, fmt.Errorf("parse CLINIC_REFUND_PARTIAL_WINDOW: %w", err)
		}
		policy.PartialWindow = v
	}

	if raw := strings.TrimSpace(os.Getenv("CLINIC_REFUND_PARTIAL_PERCENT")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return repository.ClinicBookingRefundPolicy{}, fmt.Errorf("parse CLINIC_REFUND_PARTIAL_PERCENT: %w", err)
		}
		policy.PartialPercent = v
	}

	normalized, err := repository.NormalizeClinicBookingRefundPolicy(policy)
	if err != nil {
		return repository.ClinicBookingRefundPolicy{}, err
	}
	return normalized, nil
}
