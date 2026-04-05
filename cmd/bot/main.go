package main

import (
	"database/sql"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/bot"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/cache"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/dbconfig"
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

	bookingRepo, err := buildBookingRepository(logger)
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

	h := bot.Handlers{
		Bot:         tg,
		Logger:      logger,
		Booking:     bookingService,
		BotUsername: tg.Self.UserName,
	}

	logger.Info("bot started with long polling, press Ctrl+C to stop")

	for {
		select {
		case update := <-updates:
			applyTelegramUpdate(&h, update)

		case sig := <-stop:
			logger.Info("received signal, shutting down", "signal", sig.String())
			return
		}
	}
}

func buildBookingRepository(logger slogLogger) (repository.BookingRepository, error) {
	dsn, err := dbconfig.ResolveDSN()
	if err != nil {
		return nil, err
	}
	if dsn == "" {
		logger.Info("DB_DSN / DB_PASSWORD_FILE not set, using in-memory booking repository")
		return repository.NewMemoryRepository(), nil
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	logger.Info("postgres booking repository enabled")
	return repository.NewPostgresRepository(db), nil
}
