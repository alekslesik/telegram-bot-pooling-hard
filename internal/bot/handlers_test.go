package bot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/service"
)

type fakePayments struct {
	validateErr error
	applyErr    error
	result      repository.StarsTopUpResult
}

func (f fakePayments) BuildTopUpInvoicePayload(userID, stars int64) (string, error) {
	return "payload", nil
}

func (f fakePayments) ValidatePreCheckout(fromUserID int64, currency string, totalStars int64, payload string) error {
	return f.validateErr
}

func (f fakePayments) ApplySuccessfulPayment(fromUserID int64, sp *tgbotapi.SuccessfulPayment) (repository.StarsTopUpResult, error) {
	return f.result, f.applyErr
}

type fakeBot struct {
	last tgbotapi.Chattable
}

func (f *fakeBot) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	f.last = c
	return tgbotapi.Message{}, nil
}

func (f *fakeBot) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	f.last = c
	return &tgbotapi.APIResponse{Ok: true}, nil
}

type fakeBotSendErr struct{}

func (fakeBotSendErr) Send(tgbotapi.Chattable) (tgbotapi.Message, error) {
	return tgbotapi.Message{}, errors.New("send failed")
}

func (fakeBotSendErr) Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}

type fakeBotRequestErr struct{}

func (fakeBotRequestErr) Send(tgbotapi.Chattable) (tgbotapi.Message, error) {
	return tgbotapi.Message{}, nil
}

func (fakeBotRequestErr) Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return nil, errors.New("request failed")
}

func newTestHandlers(bot TelegramClient) Handlers {
	return Handlers{
		Bot:    bot,
		Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})),
	}
}

func TestHandlers_HandleMessage_Echo(t *testing.T) {
	fb := &fakeBot{}
	h := newTestHandlers(fb)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 123},
		Text: "hello",
	}

	h.HandleMessage(msg)

	cfg, ok := fb.last.(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", fb.last)
	}

	if cfg.ChatID != msg.Chat.ID {
		t.Errorf("expected ChatID %d, got %d", msg.Chat.ID, cfg.ChatID)
	}
	if cfg.Text != "Ты написал: "+msg.Text {
		t.Errorf("unexpected text: %q", cfg.Text)
	}
}

func TestHandlers_HandleCommand_Start(t *testing.T) {
	fb := &fakeBot{}
	h := newTestHandlers(fb)

	text := "/start"
	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 42},
		Text: text,
		Entities: []tgbotapi.MessageEntity{{
			Type:   "bot_command",
			Offset: 0,
			Length: len(text),
		}},
	}

	h.HandleCommand(msg)

	cfg, ok := fb.last.(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", fb.last)
	}
	if cfg.Text == "" {
		t.Fatalf("expected non-empty /start reply")
	}
	if _, ok := cfg.ReplyMarkup.(tgbotapi.ReplyKeyboardMarkup); !ok {
		t.Fatalf("expected reply keyboard markup for /start")
	}
}

func TestHandlers_HandleCommand_Echo_Args(t *testing.T) {
	fb := &fakeBot{}
	h := newTestHandlers(fb)

	text := "/echo hello"
	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 77},
		Text: text,
		Entities: []tgbotapi.MessageEntity{{
			Type:   "bot_command",
			Offset: 0,
			Length: len("/echo"),
		}},
	}

	h.HandleCommand(msg)

	cfg, ok := fb.last.(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", fb.last)
	}
	if cfg.Text != "hello" {
		t.Fatalf("expected echoed args %q, got %q", "hello", cfg.Text)
	}
}

func TestHandlers_HandleMessage_ButtonMappedToCommand(t *testing.T) {
	fb := &fakeBot{}
	h := newTestHandlers(fb)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 555},
		Text: "🆘 Помощь",
	}

	h.HandleMessage(msg)

	cfg, ok := fb.last.(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", fb.last)
	}
	if !strings.Contains(cfg.Text, "Что я умею") {
		t.Fatalf("expected help response, got %q", cfg.Text)
	}
}

func commandMessage(chatID int64, fullText string, commandLen int) *tgbotapi.Message {
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: chatID},
		Text: fullText,
		Entities: []tgbotapi.MessageEntity{{
			Type:   "bot_command",
			Offset: 0,
			Length: commandLen,
		}},
	}
}

func TestRenderUseCases(t *testing.T) {
	s := renderUseCases()
	if !strings.Contains(s, "Салон") || !strings.Contains(s, "Идея простая") {
		t.Fatalf("unexpected use cases text: %q", s)
	}
}

func TestDemoInlineMenuKeyboard(t *testing.T) {
	k := demoInlineMenuKeyboard()
	if len(k.InlineKeyboard) != 5 {
		t.Fatalf("expected 5 inline rows, got %d", len(k.InlineKeyboard))
	}
}

func TestAdminKeyboard_VisibilityByCapabilities(t *testing.T) {
	h := newTestHandlers(&fakeBot{})
	owner := service.AdminCapabilities{
		CanOpenPanel:      true,
		CanManageCatalog:  true,
		CanManageDaySlots: true,
		CanViewAnalytics:  true,
		CanManageAdmins:   true,
		CanManageBlackout: true,
	}
	keyboard := h.adminKeyboard(owner)
	var data []string
	for _, row := range keyboard.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData != nil {
				data = append(data, *btn.CallbackData)
			}
		}
	}
	joined := strings.Join(data, ",")
	for _, expected := range []string{"admin:slotsrange", "admin:closedays", "admin:opendays", "admin:blackout", "admin:adminupsert"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected callback %s in keyboard, got %s", expected, joined)
		}
	}
}

func TestHandlers_HandleCommand_AllRegistered(t *testing.T) {
	tests := []struct {
		cmd      string
		fullText string
		cmdLen   int
		contains string
	}{
		{"start", "/start", 6, "сервисов с записью"},
		{"help", "/help", 5, "Что я умею"},
		{"ping", "/ping", 5, "pong"},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			fb := &fakeBot{}
			h := newTestHandlers(fb)
			h.HandleCommand(commandMessage(1, tt.fullText, tt.cmdLen))
			cfg, ok := fb.last.(tgbotapi.MessageConfig)
			if !ok {
				t.Fatalf("expected MessageConfig, got %T", fb.last)
			}
			if !strings.Contains(strings.ToLower(cfg.Text), strings.ToLower(tt.contains)) {
				t.Errorf("reply %q should contain %q", cfg.Text, tt.contains)
			}
			if _, ok := cfg.ReplyMarkup.(tgbotapi.ReplyKeyboardMarkup); !ok {
				t.Fatalf("expected reply keyboard, got %T", cfg.ReplyMarkup)
			}
		})
	}
}

func TestHandlers_HandleCommand_Unknown(t *testing.T) {
	fb := &fakeBot{}
	h := newTestHandlers(fb)
	h.HandleCommand(commandMessage(9, "/nope", 5))
	cfg, ok := fb.last.(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", fb.last)
	}
	if !strings.Contains(cfg.Text, "Неизвестная") {
		t.Fatalf("expected unknown command reply, got %q", cfg.Text)
	}
}

func TestHandlers_HandleCommand_Echo_NoArgs(t *testing.T) {
	fb := &fakeBot{}
	h := newTestHandlers(fb)
	h.HandleCommand(commandMessage(3, "/echo", 5))
	cfg, ok := fb.last.(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", fb.last)
	}
	if !strings.Contains(cfg.Text, "Использование") {
		t.Fatalf("expected usage hint, got %q", cfg.Text)
	}
}

func TestHandlers_HandleMessage_DelegatesCommand(t *testing.T) {
	fb := &fakeBot{}
	h := newTestHandlers(fb)
	h.HandleMessage(commandMessage(8, "/start", 6))
	cfg, ok := fb.last.(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", fb.last)
	}
	if !strings.Contains(cfg.Text, "Привет") {
		t.Fatalf("expected /start reply, got %q", cfg.Text)
	}
}

func TestHandlers_sendCommandReply_SendError(t *testing.T) {
	h := newTestHandlers(fakeBotSendErr{})
	h.HandleCommand(commandMessage(1, "/ping", 5))
}

func TestHandlers_HandleMessage_SendError(t *testing.T) {
	h := newTestHandlers(fakeBotSendErr{})
	h.HandleMessage(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1}, Text: "x"})
}

func TestHandlers_HandleCommand_Unknown_SendError(t *testing.T) {
	h := newTestHandlers(fakeBotSendErr{})
	h.HandleCommand(commandMessage(1, "/unknown", 8))
}

func TestHandlers_HandleCallback_Nil(t *testing.T) {
	h := newTestHandlers(&fakeBot{})
	h.HandleCallback(nil)
}

func TestHandlers_HandleCallback_NoMessage(t *testing.T) {
	h := newTestHandlers(&fakeBot{})
	h.HandleCallback(&tgbotapi.CallbackQuery{ID: "1", Message: nil})
}

func TestHandlers_HandleCallback_UnknownData(t *testing.T) {
	fb := &fakeBot{}
	h := newTestHandlers(fb)
	h.HandleCallback(&tgbotapi.CallbackQuery{
		ID: "cb1",
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 10},
		},
		Data: "other",
	})
	if _, ok := fb.last.(tgbotapi.CallbackConfig); !ok {
		t.Fatalf("unknown callback should answer callback only, got %T", fb.last)
	}
}

func TestHandlers_HandleCallback_UnknownData_RequestError(t *testing.T) {
	h := newTestHandlers(fakeBotRequestErr{})
	h.HandleCallback(&tgbotapi.CallbackQuery{
		ID: "cb1",
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 10},
		},
		Data: "other",
	})
}

func TestHandlers_HandleCallback_CmdStart(t *testing.T) {
	fb := &fakeBot{}
	h := newTestHandlers(fb)
	h.HandleCallback(&tgbotapi.CallbackQuery{
		ID:   "cb2",
		From: &tgbotapi.User{ID: 99},
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 10},
		},
		Data: "cmd:start",
	})
	cfg, ok := fb.last.(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", fb.last)
	}
	if !strings.Contains(cfg.Text, "Привет") {
		t.Fatalf("expected start text, got %q", cfg.Text)
	}
}

func TestHandlers_HandleCallback_RequestErrorOnAnswer(t *testing.T) {
	h := newTestHandlers(fakeBotRequestErr{})
	h.HandleCallback(&tgbotapi.CallbackQuery{
		ID: "cb3",
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 10},
		},
		Data: "cmd:ping",
	})
}

func TestHandlers_HandleCallback_SendErrorAfterAnswer(t *testing.T) {
	h := newTestHandlers(fakeBotSendErr{})
	h.HandleCallback(&tgbotapi.CallbackQuery{
		ID: "cb4",
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 10},
		},
		Data: "cmd:ping",
	})
}

func TestHandlers_BookingFlow(t *testing.T) {
	fb := &fakeBot{}
	repo := repository.NewMemoryRepository()
	h := newTestHandlers(fb)
	h.Booking = service.NewBookingService(repo, nil)

	h.HandleCommand(commandMessage(1, "/book", 5))
	cfg, ok := fb.last.(tgbotapi.MessageConfig)
	if !ok || !strings.Contains(cfg.Text, "ФИО") {
		t.Fatalf("unexpected /book response: %T %+v", fb.last, cfg)
	}

	handled, msg, err := h.Booking.HandleText(context.Background(), 1, "Ivan Ivanov")
	if err != nil || !handled || !strings.Contains(msg, "phone number") {
		t.Fatalf("name step failed: handled=%v err=%v msg=%q", handled, err, msg)
	}
	handled, msg, err = h.Booking.HandleText(context.Background(), 1, "+79991234567")
	if err != nil || !handled || !strings.Contains(msg, "Профиль сохранен") {
		t.Fatalf("phone step failed: handled=%v err=%v msg=%q", handled, err, msg)
	}
	handled, msg, err = h.Booking.HandleText(context.Background(), 1, "1")
	if err != nil || handled || msg != "" {
		t.Fatalf("expected registration flow completed: handled=%v err=%v msg=%q", handled, err, msg)
	}
}

func TestHandlers_HandlePreCheckout_Reject(t *testing.T) {
	fb := &fakeBot{}
	h := newTestHandlers(fb)
	h.Payments = fakePayments{validateErr: errors.New("bad payload")}
	h.HandlePreCheckout(&tgbotapi.PreCheckoutQuery{
		ID:             "pcq1",
		Currency:       "XTR",
		TotalAmount:    10,
		InvoicePayload: "bad",
		From:           &tgbotapi.User{ID: 7, LanguageCode: "en"},
	})
	cfg, ok := fb.last.(tgbotapi.PreCheckoutConfig)
	if !ok {
		t.Fatalf("expected PreCheckoutConfig, got %T", fb.last)
	}
	if cfg.OK {
		t.Fatal("expected rejected pre-checkout")
	}
	if cfg.ErrorMessage == "" {
		t.Fatal("expected localized error message")
	}
}

func TestHandlers_HandleMessage_SuccessfulPayment_SendsConfirmation(t *testing.T) {
	fb := &fakeBot{}
	h := newTestHandlers(fb)
	h.Payments = fakePayments{result: repository.StarsTopUpResult{BalanceAfter: 1234}}
	h.HandleMessage(&tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		From: &tgbotapi.User{ID: 100, LanguageCode: "en"},
		SuccessfulPayment: &tgbotapi.SuccessfulPayment{
			Currency:       "XTR",
			TotalAmount:    15,
			InvoicePayload: "p",
		},
	})
	cfg, ok := fb.last.(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", fb.last)
	}
	if !strings.Contains(cfg.Text, "Новый баланс") {
		t.Fatalf("expected payment success confirmation, got %q", cfg.Text)
	}
}

func TestHandlers_HandleBookingCallback_LogsFunnelSpecialtySelected(t *testing.T) {
	fb := &fakeBot{}
	repo := repository.NewMemoryRepository()
	h := newTestHandlers(fb)
	h.Booking = service.NewBookingService(repo, nil)

	h.handleBookingCallback(&tgbotapi.CallbackQuery{
		Data: "book:spec:12:0",
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 10},
		},
		From: &tgbotapi.User{ID: 77},
	})

	counts, err := repo.CountAnalyticsByEventSince(context.Background(), time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("count analytics: %v", err)
	}
	if got := counts["funnel_book_specialty_selected"]; got != 1 {
		t.Fatalf("expected funnel_book_specialty_selected=1, got %d", got)
	}
}

func TestHandlers_HandleBookingCallback_LogsFunnelDoctorSelected(t *testing.T) {
	fb := &fakeBot{}
	repo := repository.NewMemoryRepository()
	h := newTestHandlers(fb)
	h.Booking = service.NewBookingService(repo, nil)

	h.handleBookingCallback(&tgbotapi.CallbackQuery{
		Data: "book:doc:12:34:0",
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 10},
		},
		From: &tgbotapi.User{ID: 77},
	})

	counts, err := repo.CountAnalyticsByEventSince(context.Background(), time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("count analytics: %v", err)
	}
	if got := counts["funnel_book_doctor_selected"]; got != 1 {
		t.Fatalf("expected funnel_book_doctor_selected=1, got %d", got)
	}
}

func TestHandlers_HandleBookingCallback_LogsFunnelSlotSelected(t *testing.T) {
	fb := &fakeBot{}
	repo := repository.NewMemoryRepository()
	h := newTestHandlers(fb)
	h.Booking = service.NewBookingService(repo, nil)

	h.handleBookingCallback(&tgbotapi.CallbackQuery{
		Data: "book:slot:12:34:56",
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 10},
		},
		From: &tgbotapi.User{ID: 77},
	})

	counts, err := repo.CountAnalyticsByEventSince(context.Background(), time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("count analytics: %v", err)
	}
	if got := counts["funnel_book_slot_selected"]; got != 1 {
		t.Fatalf("expected funnel_book_slot_selected=1, got %d", got)
	}
}

func TestHandlers_HandleAdminCallback_AnalyticsPeriods(t *testing.T) {
	fb := &fakeBot{}
	repo := repository.NewMemoryRepository()
	h := newTestHandlers(fb)
	h.Booking = service.NewBookingService(repo, nil)
	repo.SetAdminRole(77, repository.AdminRoleAdmin)

	cases := []string{"admin:analytics", "admin:analytics:30"}
	for _, data := range cases {
		h.handleAdminCallback(&tgbotapi.CallbackQuery{
			Data: data,
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 10},
			},
			From: &tgbotapi.User{ID: 77},
		})

		cfg, ok := fb.last.(tgbotapi.MessageConfig)
		if !ok {
			t.Fatalf("expected MessageConfig, got %T", fb.last)
		}
		if !strings.Contains(cfg.Text, "outbox_pending") {
			t.Fatalf("expected analytics report text for %q, got %q", data, cfg.Text)
		}
	}
}

func TestHandlers_AdminKeyboard_AnalyticsPeriodButtons(t *testing.T) {
	h := newTestHandlers(&fakeBot{})
	kb := h.adminKeyboard(service.AdminCapabilities{CanViewAnalytics: true})
	if kb == nil {
		t.Fatalf("expected keyboard")
	}

	var has7, has30 bool
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == nil {
				continue
			}
			if *btn.CallbackData == "admin:analytics:7" {
				has7 = true
			}
			if *btn.CallbackData == "admin:analytics:30" {
				has30 = true
			}
		}
	}
	if !has7 || !has30 {
		t.Fatalf("expected analytics period buttons, got has7=%v has30=%v", has7, has30)
	}
}
