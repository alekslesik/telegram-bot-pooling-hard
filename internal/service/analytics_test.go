package service

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBuildAnalyticsEnvelopePayloadValidContext(t *testing.T) {
	uid := int64(42)
	raw := buildAnalyticsEnvelopePayload(&uid, "booking_confirmed", `{"booking_id":1}`, "outbox-worker", time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC))
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["event_name"] != "booking_confirmed" {
		t.Fatalf("unexpected event_name: %v", payload["event_name"])
	}
	if payload["source"] != "outbox-worker" {
		t.Fatalf("unexpected source: %v", payload["source"])
	}
	if payload["ts"] == "" {
		t.Fatalf("expected ts to be set")
	}
	ctxObj, ok := payload["context"].(map[string]any)
	if !ok {
		t.Fatalf("context should be object, got %T", payload["context"])
	}
	if _, ok := ctxObj["booking_id"]; !ok {
		t.Fatalf("expected context.booking_id")
	}
}

func TestBuildAnalyticsEnvelopePayloadInvalidContextFallback(t *testing.T) {
	raw := buildAnalyticsEnvelopePayload(nil, "custom_event", "{broken-json", "", time.Now().UTC())
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["source"] != "bot" {
		t.Fatalf("expected default source 'bot', got %v", payload["source"])
	}
	ctxObj, ok := payload["context"].(map[string]any)
	if !ok {
		t.Fatalf("context should be object, got %T", payload["context"])
	}
	if _, ok := ctxObj["raw"]; !ok {
		t.Fatalf("expected fallback raw context field")
	}
}
