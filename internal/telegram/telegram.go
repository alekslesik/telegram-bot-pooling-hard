package telegram

import (
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot interface {
	Send(tgbotapi.Chattable) (tgbotapi.Message, error)
	GetUpdatesChan(tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel
}

// defaultHTTPClient builds the client used by New(); tests may replace it to avoid real HTTP.
var defaultHTTPClient = func() tgbotapi.HTTPClient {
	return &http.Client{}
}

func New(token string) (*tgbotapi.BotAPI, error) {
	return NewWithHTTPClient(token, defaultHTTPClient())
}

// NewWithHTTPClient creates a BotAPI using the given HTTP client (injectable for tests).
func NewWithHTTPClient(token string, client tgbotapi.HTTPClient) (*tgbotapi.BotAPI, error) {
	return tgbotapi.NewBotAPIWithClient(token, tgbotapi.APIEndpoint, client)
}
