package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"text/template"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/i18n"
	"github.com/alekslesik/telegram-bot-pooling-hard/internal/service"
)

const inlinePageSize = 4

// TelegramClient — минимум для Send и ответа на callback (answerCallbackQuery).
type TelegramClient interface {
	Send(tgbotapi.Chattable) (tgbotapi.Message, error)
	Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

type Handlers struct {
	Bot         TelegramClient
	Logger      *slog.Logger
	Booking     *service.BookingService
	BotUsername string
}

type Command struct {
	Name        string
	Description string
	ParseMode   string
	BuildText   func(msg *tgbotapi.Message) string
}

type UseCaseCategory struct {
	Title string
	Items []string
}

var commandButtons = map[string]string{
	"🚀 Старт":              "start",
	"🗓️ Записаться":        "book",
	"📅 Мои записи":         "mybookings",
	"❌ Отмена записи":      "cancelbooking",
	"📤 Загрузить документ": "uploaddoc",
	"💼 Кабинет":            "cabinet",
	"🇬🇧 EN":                "lang_en",
	"🇷🇺 RU":                "lang_ru",
	"🛠️ Админ":             "admin",
	"🆘 Помощь":             "help",
}

// demoInlineMenuKeyboard — те же пункты, что reply-клавиатура и меню у поля ввода.
func demoInlineMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🚀 Старт", "cmd:start"),
			tgbotapi.NewInlineKeyboardButtonData("🗓️ Записаться", "cmd:book"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Демо-меню", "cmd:menu"),
			tgbotapi.NewInlineKeyboardButtonData("🆘 Помощь", "cmd:help"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("ℹ️ О боте", "cmd:about"),
			tgbotapi.NewInlineKeyboardButtonData("💼 Примеры задач", "cmd:usecases"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🧩 Возможности", "cmd:features"),
			tgbotapi.NewInlineKeyboardButtonData("✅ Проверка статуса", "cmd:ping"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗣️ Повторить текст", "cmd:echo"),
		),
	)
}

func commandKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🚀 Старт"),
			tgbotapi.NewKeyboardButton("🗓️ Записаться"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("❌ Отмена записи"),
			tgbotapi.NewKeyboardButton("📅 Мои записи"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📤 Загрузить документ"),
			tgbotapi.NewKeyboardButton("🆘 Помощь"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("💼 Кабинет"),
			tgbotapi.NewKeyboardButton("🇬🇧 EN"),
			tgbotapi.NewKeyboardButton("🇷🇺 RU"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🛠️ Админ"),
		),
	)
}

var useCases = []UseCaseCategory{
	{
		Title: "Салон / студия / услуги",
		Items: []string{
			"рассказать про услуги и цены",
			"принять заявку или запись",
			"отправить напоминание перед визитом",
		},
	},
	{
		Title: "Онлайн‑курсы / эксперты",
		Items: []string{
			"выдать материалы и инструкции",
			"собрать вопросы от учеников",
			"аккуратно предлагать доп. продукты",
		},
	},
	{
		Title: "Малый бизнес",
		Items: []string{
			"ответы на частые вопросы",
			"получение контакта для звонка",
			"быстрые опросы клиентов",
		},
	},
}

var usecasesTmpl = template.Must(template.New("usecases").Funcs(template.FuncMap{
	"add1": func(i int) int { return i + 1 },
}).Parse(
	`*Примеры задач, для которых подходит такой бот:*

{{- range $i, $c := . }}
{{ add1 $i }}. {{ $c.Title }}:
{{- range $c.Items }}
   — {{ . }}
{{- end }}

{{- end }}
Идея простая: всё, что менеджер делает руками в переписке, можно постепенно перенести в бота.`,
))

func renderUseCases() string {
	var buf bytes.Buffer
	_ = usecasesTmpl.Execute(&buf, useCases)
	return buf.String()
}

func (h Handlers) commandRegistry() map[string]Command {
	commands := map[string]Command{
		"start": {
			Name:        "start",
			Description: "приветствие и сценарии для сервиса записи",
			BuildText: func(_ *tgbotapi.Message) string {
				return "Привет! Я бот для сервисов с записью на прием.\n\n" +
					"Подходит для демонстрации клиники, частного кабинета, салона, студии, консультаций и других услуг по времени.\n\n" +
					"Что умеет эта версия:\n" +
					"- регистрация клиента\n" +
					"- запись на прием\n" +
					"- отмена записи\n" +
					"- просмотр моих записей\n" +
					"- загрузка документов.\n\n" +
					"Нажми «🗓️ Записаться» или используй /book."
			},
		},
		"book": {
			Name:        "book",
			Description: "начать запись на услугу (wizard)",
			BuildText: func(_ *tgbotapi.Message) string {
				return "Starting booking flow..."
			},
		},
		"ping": {
			Name:        "ping",
			Description: "проверка, что бот онлайн",
			BuildText: func(_ *tgbotapi.Message) string {
				return "pong ✅ Бот запущен и готов работать с клиентами."
			},
		},
		"admin": {
			Name:        "admin",
			Description: "админ-панель для управления расписанием",
			BuildText: func(_ *tgbotapi.Message) string {
				return "Открываю админ-панель..."
			},
		},
		"echo": {
			Name:        "echo",
			Description: "повторить ваш текст (пример простой команды)",
			BuildText: func(msg *tgbotapi.Message) string {
				args := strings.TrimSpace(msg.CommandArguments())
				if args == "" {
					return "Использование: /echo <текст, который нужно повторить>"
				}
				return args
			},
		},
	}

	commands["help"] = Command{
		Name:        "help",
		Description: "это сообщение с возможностями",
		ParseMode:   tgbotapi.ModeMarkdown,
		BuildText: func(_ *tgbotapi.Message) string {
			lines := []string{
				"Я бот, который помогает автоматизировать общение с клиентами.\n",
				"*Что я умею прямо сейчас:*",
			}

			order := []string{"start", "book", "admin", "help", "ping", "echo"}
			for _, name := range order {
				c := commands[name]
				label := "/" + c.Name
				if c.Name == "echo" {
					label = "/echo <текст>"
				}
				lines = append(lines, label+" — "+c.Description)
			}

			lines = append(lines, "/cancel — отменить активный сценарий записи")
			lines = append(lines, "", "Если просто написать сообщение — я отвечу тем же текстом. Это демонстрирует, как бот может принимать и обрабатывать любые обращения клиентов.")
			return strings.Join(lines, "\n")
		},
	}

	return commands
}

func (h Handlers) HandleMessage(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	if msg.IsCommand() {
		h.HandleCommand(msg)
		return
	}

	if cmdName, ok := commandButtons[strings.TrimSpace(msg.Text)]; ok {
		h.sendCommandReply(chatID, cmdName, msg)
		return
	}

	if h.Booking != nil {
		if msg.Document != nil {
			h.handleIncomingDocument(chatID, telegramUserID(msg), msg.Document.FileID, msg.Document.FileName, msg.Document.MimeType, msg.Document.FileSize)
			return
		}
		if len(msg.Photo) > 0 {
			photo := msg.Photo[len(msg.Photo)-1]
			h.handleIncomingDocument(chatID, telegramUserID(msg), photo.FileID, "photo_"+photo.FileID+".jpg", "image/jpeg", photo.FileSize)
			return
		}
	}

	if h.Booking != nil {
		handled, replyText, err := h.Booking.HandleText(context.Background(), telegramUserID(msg), msg.Text)
		if err != nil {
			h.Logger.Error("booking flow failed", "err", err)
		}
		if handled {
			reply := tgbotapi.NewMessage(chatID, replyText)
			reply.ReplyMarkup = commandKeyboard()
			if _, err := h.Bot.Send(reply); err != nil {
				h.Logger.Error("failed to send booking reply", "err", err)
			}
			return
		}
	}

	reply := tgbotapi.NewMessage(chatID, "Ты написал: "+msg.Text)
	reply.ReplyMarkup = commandKeyboard()
	if _, err := h.Bot.Send(reply); err != nil {
		h.Logger.Error("failed to send message", "err", err)
	}
}

func (h Handlers) HandleCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	h.sendCommandReply(chatID, msg.Command(), msg)
}

func (h Handlers) bundleForUser(ctx context.Context, msg *tgbotapi.Message) i18n.Bundle {
	uid := telegramUserID(msg)
	stored := "ru"
	if h.Booking != nil {
		if s, err := h.Booking.PreferredLang(ctx, uid); err == nil {
			stored = s
		}
	}
	langCode := ""
	if msg != nil && msg.From != nil {
		langCode = msg.From.LanguageCode
	}
	return i18n.Bundle{Lang: i18n.Resolve(stored, langCode)}
}

func (h Handlers) sendCommandReply(chatID int64, cmdName string, msg *tgbotapi.Message) {
	ctx := context.Background()
	if h.Booking != nil && cmdName == "start" {
		uid := telegramUserID(msg)
		if err := h.Booking.EnsureUserStart(ctx, uid, msg.CommandArguments()); err != nil {
			h.Logger.Error("ensure user start failed", "err", err)
		}
		uidp := uid
		payload, _ := json.Marshal(map[string]string{"args": msg.CommandArguments()})
		_ = h.Booking.LogAnalytics(ctx, &uidp, "cmd_start", string(payload))
	}
	if h.Booking != nil && cmdName == "cabinet" {
		bal, code, err := h.Booking.AccountCabinet(ctx, telegramUserID(msg))
		if err != nil {
			h.Logger.Error("account cabinet failed", "err", err)
			reply := tgbotapi.NewMessage(chatID, "Не удалось загрузить кабинет.")
			reply.ReplyMarkup = commandKeyboard()
			_, _ = h.Bot.Send(reply)
			return
		}
		b := h.bundleForUser(ctx, msg)
		botUser := strings.TrimSpace(os.Getenv("USERNAME"))
		if h.BotUsername != "" {
			botUser = strings.TrimPrefix(h.BotUsername, "@")
		}
		reply := tgbotapi.NewMessage(chatID, b.Cabinet(bal, code, botUser))
		reply.ReplyMarkup = commandKeyboard()
		_, _ = h.Bot.Send(reply)
		return
	}
	if h.Booking != nil && (cmdName == "lang_en" || cmdName == "lang_ru") {
		lang := "en"
		newLang := i18n.En
		if cmdName == "lang_ru" {
			lang = "ru"
			newLang = i18n.Ru
		}
		if err := h.Booking.SetPreferredLang(ctx, telegramUserID(msg), lang); err != nil {
			h.Logger.Error("set preferred lang failed", "err", err)
		}
		b := i18n.Bundle{Lang: newLang}
		reply := tgbotapi.NewMessage(chatID, b.LanguageSet(strings.ToUpper(lang)))
		reply.ReplyMarkup = commandKeyboard()
		_, _ = h.Bot.Send(reply)
		return
	}
	if h.Booking != nil && cmdName == "uploaddoc" {
		text, err := h.Booking.StartDocumentUpload(context.Background(), telegramUserID(msg))
		if err != nil {
			h.Logger.Error("failed to start document upload", "err", err)
			text = "Не удалось начать загрузку. Попробуйте позже."
		}
		reply := tgbotapi.NewMessage(chatID, text)
		reply.ReplyMarkup = commandKeyboard()
		if _, err := h.Bot.Send(reply); err != nil {
			h.Logger.Error("failed to send upload-doc prompt", "err", err)
		}
		return
	}
	if h.Booking != nil && cmdName == "admin" {
		ok, text, err := h.Booking.StartAdmin(context.Background(), telegramUserID(msg))
		if err != nil {
			h.Logger.Error("failed to open admin panel", "err", err)
			text = "Не удалось открыть админ-панель."
		}
		reply := tgbotapi.NewMessage(chatID, text)
		if ok {
			reply.ReplyMarkup = h.adminKeyboard()
		} else {
			reply.ReplyMarkup = commandKeyboard()
		}
		if _, err := h.Bot.Send(reply); err != nil {
			h.Logger.Error("failed to send admin reply", "err", err)
		}
		return
	}
	if h.Booking != nil && cmdName == "cancelbooking" {
		reply := tgbotapi.NewMessage(chatID, "Выберите запись для отмены:")
		reply.ReplyMarkup = h.cancelBookingsKeyboard(context.Background(), telegramUserID(msg), 0)
		if _, err := h.Bot.Send(reply); err != nil {
			h.Logger.Error("failed to send cancel-booking menu", "err", err)
		}
		return
	}
	if h.Booking != nil && cmdName == "mybookings" {
		reply := tgbotapi.NewMessage(chatID, "Ваши ближайшие записи:")
		reply.ReplyMarkup = h.myBookingsKeyboard(context.Background(), telegramUserID(msg), 0)
		if _, err := h.Bot.Send(reply); err != nil {
			h.Logger.Error("failed to send my-bookings menu", "err", err)
		}
		return
	}
	if h.Booking != nil && (cmdName == "book" || cmdName == "cancel") {
		var (
			replyText string
			err       error
		)
		if cmdName == "book" {
			replyText, err = h.Booking.Start(context.Background(), telegramUserID(msg))
		} else {
			replyText, err = h.Booking.Cancel(context.Background(), telegramUserID(msg))
		}
		if err != nil {
			h.Logger.Error("booking command failed", "cmd", cmdName, "err", err)
			replyText = "Booking command failed. Please try again."
		}
		reply := tgbotapi.NewMessage(chatID, replyText)
		if cmdName == "book" {
			registered, regErr := h.Booking.IsRegistered(context.Background(), telegramUserID(msg))
			if regErr != nil {
				h.Logger.Error("failed to check registration", "err", regErr)
			}
			if registered {
				reply.ReplyMarkup = h.specialtiesKeyboard(context.Background(), 0)
			} else {
				reply.ReplyMarkup = commandKeyboard()
			}
		} else {
			reply.ReplyMarkup = commandKeyboard()
		}
		if _, err := h.Bot.Send(reply); err != nil {
			h.Logger.Error("failed to send booking command reply", "cmd", cmdName, "err", err)
		}
		return
	}

	cmd, ok := h.commandRegistry()[cmdName]
	if !ok {
		reply := tgbotapi.NewMessage(chatID, "Неизвестная команда. Напиши /help, чтобы узнать, что я умею.")
		reply.ReplyMarkup = commandKeyboard()
		if _, err := h.Bot.Send(reply); err != nil {
			h.Logger.Error("failed to send unknown command reply", "err", err)
		}
		return
	}

	reply := tgbotapi.NewMessage(chatID, cmd.BuildText(msg))
	if cmd.ParseMode != "" {
		reply.ParseMode = cmd.ParseMode
	}
	reply.ReplyMarkup = commandKeyboard()
	if _, err := h.Bot.Send(reply); err != nil {
		h.Logger.Error("failed to send command reply", "cmd", cmdName, "err", err)
	}
}

func telegramUserID(msg *tgbotapi.Message) int64 {
	if msg != nil && msg.From != nil {
		return msg.From.ID
	}
	if msg != nil && msg.Chat != nil {
		return msg.Chat.ID
	}
	return 0
}

// HandleCallback — нажатия на inline-кнопки (те же команды, что в основном меню).
func (h Handlers) HandleCallback(q *tgbotapi.CallbackQuery) {
	if q == nil || q.Message == nil {
		return
	}
	data := strings.TrimSpace(q.Data)
	if strings.HasPrefix(data, "admin:") {
		if _, err := h.Bot.Request(tgbotapi.NewCallback(q.ID, "")); err != nil {
			h.Logger.Error("failed to answer admin callback", "err", err)
		}
		h.handleAdminCallback(q)
		return
	}
	if strings.HasPrefix(data, "my:") {
		if _, err := h.Bot.Request(tgbotapi.NewCallback(q.ID, "")); err != nil {
			h.Logger.Error("failed to answer my-bookings callback", "err", err)
		}
		h.handleMyBookingsCallback(q)
		return
	}
	if strings.HasPrefix(data, "cancel:") {
		if _, err := h.Bot.Request(tgbotapi.NewCallback(q.ID, "")); err != nil {
			h.Logger.Error("failed to answer cancel callback", "err", err)
		}
		h.handleCancelCallback(q)
		return
	}
	if strings.HasPrefix(data, "book:") {
		if _, err := h.Bot.Request(tgbotapi.NewCallback(q.ID, "")); err != nil {
			h.Logger.Error("failed to answer booking callback", "err", err)
		}
		h.handleBookingCallback(q)
		return
	}
	if !strings.HasPrefix(data, "cmd:") {
		if _, err := h.Bot.Request(tgbotapi.NewCallback(q.ID, "")); err != nil {
			h.Logger.Error("failed to answer unknown callback", "err", err)
		}
		return
	}
	cmdName := strings.TrimPrefix(data, "cmd:")
	if _, err := h.Bot.Request(tgbotapi.NewCallback(q.ID, "")); err != nil {
		h.Logger.Error("failed to answer callback", "err", err)
	}
	fake := &tgbotapi.Message{
		Chat: q.Message.Chat,
		From: q.From,
	}
	h.sendCommandReply(q.Message.Chat.ID, cmdName, fake)
}

func (h Handlers) handleMyBookingsCallback(q *tgbotapi.CallbackQuery) {
	parts := strings.Split(strings.TrimSpace(q.Data), ":")
	if len(parts) < 2 || parts[0] != "my" || h.Booking == nil {
		return
	}
	userID := q.Message.Chat.ID
	if q.From != nil {
		userID = q.From.ID
	}
	chatID := q.Message.Chat.ID
	switch parts[1] {
	case "page":
		if len(parts) != 3 {
			return
		}
		page, ok := parsePositiveInt(parts[2])
		if !ok {
			return
		}
		reply := tgbotapi.NewMessage(chatID, "Ваши ближайшие записи:")
		reply.ReplyMarkup = h.myBookingsKeyboard(context.Background(), userID, page)
		_, _ = h.Bot.Send(reply)
	case "close":
		reply := tgbotapi.NewMessage(chatID, "Ок, список записей закрыт.")
		reply.ReplyMarkup = commandKeyboard()
		_, _ = h.Bot.Send(reply)
	}
}

func (h Handlers) handleCancelCallback(q *tgbotapi.CallbackQuery) {
	parts := strings.Split(strings.TrimSpace(q.Data), ":")
	if len(parts) < 2 || parts[0] != "cancel" || h.Booking == nil {
		return
	}
	userID := q.Message.Chat.ID
	if q.From != nil {
		userID = q.From.ID
	}
	chatID := q.Message.Chat.ID
	switch parts[1] {
	case "page":
		if len(parts) != 3 {
			return
		}
		page, ok := parsePositiveInt(parts[2])
		if !ok {
			return
		}
		reply := tgbotapi.NewMessage(chatID, "Выберите запись для отмены:")
		reply.ReplyMarkup = h.cancelBookingsKeyboard(context.Background(), userID, page)
		_, _ = h.Bot.Send(reply)
	case "item":
		if len(parts) != 3 {
			return
		}
		bookingID, ok := parseInt64(parts[2])
		if !ok {
			return
		}
		text, err := h.Booking.CancelClinicBooking(context.Background(), userID, bookingID)
		if err != nil {
			h.Logger.Error("cancel clinic booking failed", "err", err)
			text = "Не удалось отменить запись. Попробуйте позже."
		}
		reply := tgbotapi.NewMessage(chatID, text)
		reply.ReplyMarkup = commandKeyboard()
		_, _ = h.Bot.Send(reply)
	case "close":
		reply := tgbotapi.NewMessage(chatID, "Ок, отмена записи закрыта.")
		reply.ReplyMarkup = commandKeyboard()
		_, _ = h.Bot.Send(reply)
	}
}

func (h Handlers) handleBookingCallback(q *tgbotapi.CallbackQuery) {
	parts := strings.Split(strings.TrimSpace(q.Data), ":")
	if len(parts) < 2 || parts[0] != "book" {
		return
	}
	chatID := q.Message.Chat.ID
	switch parts[1] {
	case "close":
		reply := tgbotapi.NewMessage(chatID, "Ок, окно записи закрыто.")
		reply.ReplyMarkup = commandKeyboard()
		_, _ = h.Bot.Send(reply)
	case "specp":
		if len(parts) != 3 {
			return
		}
		page, ok := parsePositiveInt(parts[2])
		if !ok {
			return
		}
		reply := tgbotapi.NewMessage(chatID, "Выберите направление:")
		reply.ReplyMarkup = h.specialtiesKeyboard(context.Background(), page)
		_, _ = h.Bot.Send(reply)
	case "spec":
		if len(parts) != 4 {
			return
		}
		specID, ok1 := parseInt64(parts[2])
		page, ok2 := parsePositiveInt(parts[3])
		if !ok1 || !ok2 {
			return
		}
		reply := tgbotapi.NewMessage(chatID, "Выберите врача:")
		reply.ReplyMarkup = h.doctorsKeyboard(context.Background(), specID, page)
		_, _ = h.Bot.Send(reply)
	case "docp":
		if len(parts) != 4 {
			return
		}
		specID, ok1 := parseInt64(parts[2])
		page, ok2 := parsePositiveInt(parts[3])
		if !ok1 || !ok2 {
			return
		}
		reply := tgbotapi.NewMessage(chatID, "Выберите врача:")
		reply.ReplyMarkup = h.doctorsKeyboard(context.Background(), specID, page)
		_, _ = h.Bot.Send(reply)
	case "doc":
		if len(parts) != 5 {
			return
		}
		specID, ok1 := parseInt64(parts[2])
		docID, ok2 := parseInt64(parts[3])
		page, ok3 := parsePositiveInt(parts[4])
		if !ok1 || !ok2 || !ok3 {
			return
		}
		reply := tgbotapi.NewMessage(chatID, "Выберите дату и время:")
		reply.ReplyMarkup = h.slotsKeyboard(context.Background(), specID, docID, page)
		_, _ = h.Bot.Send(reply)
	case "slotp":
		if len(parts) != 5 {
			return
		}
		specID, ok1 := parseInt64(parts[2])
		docID, ok2 := parseInt64(parts[3])
		page, ok3 := parsePositiveInt(parts[4])
		if !ok1 || !ok2 || !ok3 {
			return
		}
		reply := tgbotapi.NewMessage(chatID, "Выберите дату и время:")
		reply.ReplyMarkup = h.slotsKeyboard(context.Background(), specID, docID, page)
		_, _ = h.Bot.Send(reply)
	case "slot":
		if len(parts) != 5 || h.Booking == nil {
			return
		}
		specID, ok1 := parseInt64(parts[2])
		docID, ok2 := parseInt64(parts[3])
		slotID, ok3 := parseInt64(parts[4])
		if !ok1 || !ok2 || !ok3 {
			return
		}
		userID := q.Message.Chat.ID
		if q.From != nil {
			userID = q.From.ID
		}
		lang := i18n.Ru
		if q.From != nil {
			stored := "ru"
			if s, err := h.Booking.PreferredLang(context.Background(), userID); err == nil {
				stored = s
			}
			lang = i18n.Resolve(stored, q.From.LanguageCode)
		}
		text, err := h.Booking.ConfirmClinicBooking(context.Background(), userID, specID, docID, slotID, lang)
		var insufficient *service.InsufficientFundsError
		if errors.As(err, &insufficient) {
			b := i18n.Bundle{Lang: lang}
			text = b.InsufficientBalance(insufficient.FeeCents, insufficient.BalanceCents)
			err = nil
		}
		if err != nil {
			h.Logger.Error("confirm clinic booking failed", "err", err)
			text = "Не удалось подтвердить запись. Попробуйте позже."
		}
		if err == nil && strings.Contains(text, "\nID:") {
			uidp := userID
			payload, _ := json.Marshal(map[string]int64{"spec_id": specID, "doc_id": docID, "slot_id": slotID})
			_ = h.Booking.LogAnalytics(context.Background(), &uidp, "booking_confirmed", string(payload))
		}
		reply := tgbotapi.NewMessage(chatID, text)
		reply.ReplyMarkup = commandKeyboard()
		_, _ = h.Bot.Send(reply)
	}
}

func (h Handlers) specialtiesKeyboard(ctx context.Context, page int) *tgbotapi.InlineKeyboardMarkup {
	inline := tgbotapi.NewInlineKeyboardMarkup()
	if h.Booking == nil {
		return &inline
	}
	items, total, err := h.Booking.ListSpecialtiesPage(ctx, page, inlinePageSize)
	if err != nil {
		h.Logger.Error("list specialties failed", "err", err)
		inline.InlineKeyboard = append(inline.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Закрыть", "book:close"),
		))
		return &inline
	}
	for _, item := range items {
		inline.InlineKeyboard = append(inline.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(item.Name, fmt.Sprintf("book:spec:%d:0", item.ID)),
		))
	}
	inline.InlineKeyboard = append(inline.InlineKeyboard, pageRow("book:specp", page, total, inlinePageSize, "book:close"))
	return &inline
}

func (h Handlers) doctorsKeyboard(ctx context.Context, specialtyID int64, page int) *tgbotapi.InlineKeyboardMarkup {
	inline := tgbotapi.NewInlineKeyboardMarkup()
	items, total, err := h.Booking.ListDoctorsPage(ctx, specialtyID, page, inlinePageSize)
	if err != nil {
		h.Logger.Error("list doctors failed", "err", err)
		return &inline
	}
	for _, item := range items {
		inline.InlineKeyboard = append(inline.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(item.FullName, fmt.Sprintf("book:doc:%d:%d:0", specialtyID, item.ID)),
		))
	}
	inline.InlineKeyboard = append(inline.InlineKeyboard, pageRow("book:docp:"+fmt.Sprintf("%d", specialtyID), page, total, inlinePageSize, "book:specp:0"))
	return &inline
}

func (h Handlers) slotsKeyboard(ctx context.Context, specialtyID, doctorID int64, page int) *tgbotapi.InlineKeyboardMarkup {
	inline := tgbotapi.NewInlineKeyboardMarkup()
	items, total, err := h.Booking.ListSlotsPage(ctx, specialtyID, doctorID, page, inlinePageSize)
	if err != nil {
		h.Logger.Error("list slots failed", "err", err)
		return &inline
	}
	for _, item := range items {
		inline.InlineKeyboard = append(inline.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(item.StartAt.Format("02.01 15:04"), fmt.Sprintf("book:slot:%d:%d:%d", specialtyID, doctorID, item.ID)),
		))
	}
	inline.InlineKeyboard = append(inline.InlineKeyboard, pageRow("book:slotp:"+fmt.Sprintf("%d:%d", specialtyID, doctorID), page, total, inlinePageSize, fmt.Sprintf("book:spec:%d:0", specialtyID)))
	return &inline
}

func (h Handlers) cancelBookingsKeyboard(ctx context.Context, userID int64, page int) *tgbotapi.InlineKeyboardMarkup {
	inline := tgbotapi.NewInlineKeyboardMarkup()
	if h.Booking == nil {
		return &inline
	}
	items, total, err := h.Booking.ListClinicBookingsPage(ctx, userID, page, inlinePageSize)
	if err != nil {
		h.Logger.Error("list clinic bookings failed", "err", err)
		return &inline
	}
	if len(items) == 0 {
		inline.InlineKeyboard = append(inline.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Записей нет", "cancel:close"),
		))
		return &inline
	}
	for _, item := range items {
		label := fmt.Sprintf("ID %d | %s | %s", item.ID, item.StartAt.Format("02.01 15:04"), item.DoctorName)
		inline.InlineKeyboard = append(inline.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("cancel:item:%d", item.ID)),
		))
	}
	inline.InlineKeyboard = append(inline.InlineKeyboard, pageRow("cancel:page", page, total, inlinePageSize, "cancel:close"))
	return &inline
}

func (h Handlers) myBookingsKeyboard(ctx context.Context, userID int64, page int) *tgbotapi.InlineKeyboardMarkup {
	inline := tgbotapi.NewInlineKeyboardMarkup()
	if h.Booking == nil {
		return &inline
	}
	items, total, err := h.Booking.ListClinicBookingsPage(ctx, userID, page, inlinePageSize)
	if err != nil {
		h.Logger.Error("list my bookings failed", "err", err)
		return &inline
	}
	if len(items) == 0 {
		inline.InlineKeyboard = append(inline.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Записей нет", "my:close"),
		))
		return &inline
	}
	for _, item := range items {
		label := fmt.Sprintf("%s | %s | %s", item.StartAt.Format("02.01 15:04"), item.SpecialtyName, item.DoctorName)
		inline.InlineKeyboard = append(inline.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, "my:page:"+strconv.Itoa(page)),
		))
	}
	inline.InlineKeyboard = append(inline.InlineKeyboard, pageRow("my:page", page, total, inlinePageSize, "my:close"))
	return &inline
}

func pageRow(prefix string, page, total, pageSize int, back string) []tgbotapi.InlineKeyboardButton {
	row := []tgbotapi.InlineKeyboardButton{tgbotapi.NewInlineKeyboardButtonData("◀️ Назад", back)}
	lastPage := 0
	if total > 0 {
		lastPage = (total - 1) / pageSize
	}
	if page > 0 {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData("⬅️", fmt.Sprintf("%s:%d", prefix, page-1)))
	}
	if page < lastPage {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData("➡️", fmt.Sprintf("%s:%d", prefix, page+1)))
	}
	row = append(row, tgbotapi.NewInlineKeyboardButtonData("✖️", "book:close"))
	return row
}

func parsePositiveInt(raw string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v < 0 {
		return 0, false
	}
	return v, true
}

func parseInt64(raw string) (int64, bool) {
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func (h Handlers) handleIncomingDocument(chatID, userID int64, fileID, fileName, mimeType string, fileSize int) {
	text, err := h.Booking.SaveUploadedDocument(context.Background(), userID, fileID, fileName, mimeType, fileSize)
	if err != nil {
		h.Logger.Error("failed to save uploaded document", "err", err)
		text = "Не удалось сохранить документ. Попробуйте позже."
	}
	reply := tgbotapi.NewMessage(chatID, text)
	reply.ReplyMarkup = commandKeyboard()
	if _, err := h.Bot.Send(reply); err != nil {
		h.Logger.Error("failed to send upload result", "err", err)
	}
}

func (h Handlers) handleAdminCallback(q *tgbotapi.CallbackQuery) {
	if q == nil {
		return
	}
	parts := strings.Split(strings.TrimSpace(q.Data), ":")
	if len(parts) < 2 || parts[0] != "admin" || h.Booking == nil {
		return
	}

	// For our chat-based bot the Telegram user id is the requester.
	userID := q.Message.Chat.ID
	if q.From != nil {
		userID = q.From.ID
	}
	chatID := q.Message.Chat.ID

	var (
		text string
		err  error
	)

	switch parts[1] {
	case "addspec":
		text, err = h.Booking.StartAdminAddSpecialty(context.Background(), userID)
	case "adddoc":
		text, err = h.Booking.StartAdminAddDoctor(context.Background(), userID)
	case "link":
		text, err = h.Booking.StartAdminLinkDoctorSpecialty(context.Background(), userID)
	case "slots":
		text, err = h.Booking.StartAdminGenerateSlots(context.Background(), userID)
	case "closeday":
		text, err = h.Booking.StartAdminCloseDay(context.Background(), userID)
	case "openday":
		text, err = h.Booking.StartAdminOpenDay(context.Background(), userID)
	case "dayslots":
		text, err = h.Booking.StartAdminDaySlots(context.Background(), userID)
	case "analytics":
		report, errAn := h.Booking.AdminAnalyticsReport(context.Background(), userID)
		err = nil
		if errAn != nil {
			text = "Нет доступа к аналитике."
		} else {
			b := h.bundleForUser(context.Background(), &tgbotapi.Message{From: q.From})
			if strings.TrimSpace(report) == "" {
				text = b.NoAnalytics()
			} else {
				text = b.AnalyticsAdmin(strings.Split(strings.TrimSpace(report), "\n"))
			}
		}
	case "close":
		text = "Админ-панель закрыта."
	default:
		return
	}

	if err != nil {
		h.Logger.Error("admin action failed", "action", parts[1], "err", err)
		text = "Не удалось выполнить админ-действие."
	}

	reply := tgbotapi.NewMessage(chatID, text)
	reply.ReplyMarkup = commandKeyboard()
	if parts[1] == "close" {
		reply.ReplyMarkup = commandKeyboard()
	}
	if _, sendErr := h.Bot.Send(reply); sendErr != nil {
		h.Logger.Error("failed to send admin action reply", "err", sendErr)
	}
}

func (h Handlers) adminKeyboard() *tgbotapi.InlineKeyboardMarkup {
	inline := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Добавить специализацию", "admin:addspec"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Добавить врача", "admin:adddoc"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Привязать врач-специализация", "admin:link"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Сгенерировать слоты на день", "admin:slots"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Закрыть день", "admin:closeday"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Открыть день", "admin:openday"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Слоты на день", "admin:dayslots"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📊 Аналитика (7 дн.)", "admin:analytics"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✖️ Закрыть", "admin:close"),
		),
	)
	return &inline
}
