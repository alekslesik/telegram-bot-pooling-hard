package main

import (
	"bytes"
	"errors"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/alekslesik/telegram-bot-simple/internal/bot"
	"github.com/alekslesik/telegram-bot-simple/internal/logging"
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

func TestSetMyCommandsConfig(t *testing.T) {
	cfg := setMyCommandsConfig()
	cmds := cfg.Commands
	if len(cmds) != 8 {
		t.Fatalf("expected 8 commands, got %d", len(cmds))
	}
	if cmds[0].Command != "start" {
		t.Fatalf("first command: %+v", cmds[0])
	}
	_ = cfg
}

type stubTelegram struct {
	last tgbotapi.Chattable
}

func (s *stubTelegram) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	s.last = c
	return tgbotapi.Message{}, nil
}

func (s *stubTelegram) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	s.last = c
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

type errRegistrar struct{}

func (errRegistrar) Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return nil, errors.New("boom")
}

func TestRegisterBotCommands_errorLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewWithWriter(&buf)
	registerBotCommands(errRegistrar{}, logger)
	if !bytes.Contains(buf.Bytes(), []byte("failed to register bot commands")) {
		t.Fatalf("log: %s", buf.String())
	}
}

func TestRegisterBotCommands_ok(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewWithWriter(&buf)
	registerBotCommands(&stubTelegram{}, logger)
	if buf.Len() != 0 {
		t.Fatalf("expected no error log, got %s", buf.String())
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
