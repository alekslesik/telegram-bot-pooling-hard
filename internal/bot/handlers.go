package bot

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"text/template"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/alekslesik/telegram-bot-pooling-middle/internal/service"
)

// TelegramClient — минимум для Send и ответа на callback (answerCallbackQuery).
type TelegramClient interface {
	Send(tgbotapi.Chattable) (tgbotapi.Message, error)
	Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

type Handlers struct {
	Bot     TelegramClient
	Logger  *slog.Logger
	Booking *service.BookingService
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
	"🚀 Старт":       "start",
	"🗓️ Записаться": "book",
	"🆘 Помощь":      "help",
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
			tgbotapi.NewKeyboardButton("🆘 Помощь"),
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

			order := []string{"start", "book", "help", "ping", "echo"}
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

func (h Handlers) sendCommandReply(chatID int64, cmdName string, msg *tgbotapi.Message) {
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
		reply.ReplyMarkup = commandKeyboard()
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
