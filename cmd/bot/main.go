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
}

type commandRegistrar interface {
	Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
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

	tg, err := telegram.New(token)
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

	redisCache, err := cache.NewRedisFromEnv()
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	if redisCache != nil {
		defer func() { _ = redisCache.Close() }()
		logger.Info("redis cache enabled for specialty pages")
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

	if outboxWorkerEnabled() {
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
		go outboxWorker.Run(workerCtx, 2*time.Second)
		logger.Info("outbox worker enabled")
	}

	var healthSrv *health.Server
	if addr := healthAddrFromEnv(); addr != "" {
		healthSrv = health.NewServer(addr, dbConn, redisCache, outboxWorkerEnabled())
		go func() {
			if err := healthSrv.Start(); err != nil && err != http.ErrServerClosed {
				logger.Error("health server failed", "err", err)
			}
		}()
		logger.Info("health endpoints enabled", "addr", addr)
	}

	logger.Info("bot started with long polling, press Ctrl+C to stop")

	for {
		select {
		case update := <-updates:
			applyTelegramUpdate(&h, update)

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
