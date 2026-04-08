package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/repository"
)

func logAnalyticsWithEnvelope(ctx context.Context, repo repository.BookingRepository, userID *int64, eventName, contextJSON, source string) error {
	payload := buildAnalyticsEnvelopePayload(userID, eventName, contextJSON, source, time.Now().UTC())
	return repo.LogAnalyticsEvent(ctx, userID, eventName, payload)
}

func buildAnalyticsEnvelopePayload(userID *int64, eventName, contextJSON, source string, now time.Time) string {
	source = strings.TrimSpace(source)
	if source == "" {
		source = "bot"
	}
	contextJSON = strings.TrimSpace(contextJSON)
	if contextJSON == "" {
		contextJSON = "{}"
	}
	var contextAny any
	if err := json.Unmarshal([]byte(contextJSON), &contextAny); err != nil {
		contextAny = map[string]any{"raw": contextJSON}
	}

	envelope := map[string]any{
		"event_name": eventName,
		"user_id":    userID,
		"context":    contextAny,
		"ts":         now.Format(time.RFC3339Nano),
		"source":     source,
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
