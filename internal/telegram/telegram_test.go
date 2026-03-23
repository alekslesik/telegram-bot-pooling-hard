package telegram

import (
	"io"
	"net/http"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestNew_usesDefaultHTTPClientHook(t *testing.T) {
	orig := defaultHTTPClient
	t.Cleanup(func() { defaultHTTPClient = orig })

	const body = `{"ok":true,"result":{"id":3,"is_bot":true,"first_name":"N","username":"fromnew"}}`
	defaultHTTPClient = func() tgbotapi.HTTPClient {
		return &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			}),
		}
	}

	bot, err := New("tok")
	if err != nil {
		t.Fatal(err)
	}
	if bot.Self.UserName != "fromnew" {
		t.Fatalf("got %q", bot.Self.UserName)
	}
}

func TestNewWithHTTPClient_getMeOK(t *testing.T) {
	const body = `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"T","username":"mockbot"}}`
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	bot, err := NewWithHTTPClient("dummy-token", client)
	if err != nil {
		t.Fatal(err)
	}
	if bot.Self.UserName != "mockbot" {
		t.Fatalf("username: got %q", bot.Self.UserName)
	}
}
