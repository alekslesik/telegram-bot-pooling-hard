package i18n

import (
	"fmt"
	"strings"
)

// Lang is a supported UI language.
type Lang string

const (
	Ru Lang = "ru"
	En Lang = "en"
)

// Resolve picks stored preference, then Telegram language, then Russian.
func Resolve(stored string, telegramLangCode string) Lang {
	s := strings.TrimSpace(strings.ToLower(stored))
	if s == "en" {
		return En
	}
	if s == "ru" {
		return Ru
	}
	t := strings.TrimSpace(strings.ToLower(telegramLangCode))
	if len(t) >= 2 && t[:2] == "en" {
		return En
	}
	return Ru
}

// Bundle holds localized user-facing strings.
type Bundle struct {
	Lang Lang
}

func (b Bundle) InsufficientBalance(feeCents, balanceCents int64) string {
	switch b.Lang {
	case En:
		return fmt.Sprintf("Not enough balance. Booking costs %d¢, your balance is %d¢. Top up or earn referral bonuses.", feeCents, balanceCents)
	default:
		return fmt.Sprintf("Недостаточно средств. Запись стоит %d коп., на балансе %d коп. Пополните баланс или пригласите друзей по реферальной ссылке.", feeCents, balanceCents)
	}
}

func (b Bundle) BookingConfirmed(id int64, spec, doctor, slot string, feeCents, balanceAfter int64) string {
	switch b.Lang {
	case En:
		return fmt.Sprintf(
			"Booking confirmed.\nID: %d\nSpecialty: %s\nDoctor: %s\nTime: %s\nPaid: %d¢\nBalance: %d¢",
			id, spec, doctor, slot, feeCents, balanceAfter,
		)
	default:
		return fmt.Sprintf(
			"Запись подтверждена.\nID: %d\nНаправление: %s\nВрач: %s\nВремя: %s\nСписано: %d коп.\nБаланс: %d коп.",
			id, spec, doctor, slot, feeCents, balanceAfter,
		)
	}
}

func (b Bundle) Cabinet(balanceCents int64, referralCode, botUsername string) string {
	link := ""
	if botUsername != "" && referralCode != "" {
		link = fmt.Sprintf("https://t.me/%s?start=%s", strings.TrimPrefix(botUsername, "@"), referralCode)
	}
	switch b.Lang {
	case En:
		msg := fmt.Sprintf("Personal account\nBalance: %d¢ (demo credits)\nYour referral code: %s\n", balanceCents, referralCode)
		if link != "" {
			msg += "Invite link:\n" + link
		}
		return msg
	default:
		msg := fmt.Sprintf("Личный кабинет\nБаланс: %d коп. (демо-кредиты)\nВаш реферальный код: %s\n", balanceCents, referralCode)
		if link != "" {
			msg += "Ссылка для приглашения:\n" + link
		}
		return msg
	}
}

func (b Bundle) LanguageSet(code string) string {
	switch b.Lang {
	case En:
		return "Language: " + code
	default:
		return "Язык интерфейса: " + code
	}
}

func (b Bundle) AnalyticsAdmin(lines []string) string {
	switch b.Lang {
	case En:
		out := "Analytics (last 7 days, event counts):\n"
		for _, l := range lines {
			out += l + "\n"
		}
		return strings.TrimSpace(out)
	default:
		out := "Аналитика (последние 7 дней, число событий):\n"
		for _, l := range lines {
			out += l + "\n"
		}
		return strings.TrimSpace(out)
	}
}

func (b Bundle) PaymentSuccess(balanceCents int64) string {
	switch b.Lang {
	case En:
		return fmt.Sprintf("Payment received. New balance: %d¢.", balanceCents)
	default:
		return fmt.Sprintf("Платеж зачислен. Новый баланс: %d коп.", balanceCents)
	}
}

func (b Bundle) PaymentFailed() string {
	switch b.Lang {
	case En:
		return "Payment validation failed. Please try again."
	default:
		return "Не удалось подтвердить платеж. Попробуйте еще раз."
	}
}

func (b Bundle) PaymentTopUpPrompt() string {
	switch b.Lang {
	case En:
		return "Need more funds? Top up via Telegram Stars and repeat booking."
	default:
		return "Нужно пополнение? Пополните баланс через Telegram Stars и повторите запись."
	}
}

func (b Bundle) NoAnalytics() string {
	switch b.Lang {
	case En:
		return "No analytics events in this period."
	default:
		return "За этот период событий нет."
	}
}
