package repository

import (
	"strings"
	"testing"

	"github.com/lib/pq"
)

func TestIsUndefinedOutboxRelationError_MatchesOutboxEvents(t *testing.T) {
	err := &pq.Error{
		Code:    "42P01",
		Message: `relation "outbox_events" does not exist`,
	}
	if !isUndefinedOutboxRelationError(err) {
		t.Fatalf("expected outbox undefined relation error to match")
	}
}

func TestIsUndefinedOutboxRelationError_IgnoresOtherCases(t *testing.T) {
	otherRelation := &pq.Error{
		Code:    "42P01",
		Message: `relation "wallet_transactions" does not exist`,
	}
	if isUndefinedOutboxRelationError(otherRelation) {
		t.Fatalf("did not expect other relation error to match")
	}

	otherCode := &pq.Error{
		Code:    "23505",
		Message: `duplicate key value violates unique constraint`,
	}
	if isUndefinedOutboxRelationError(otherCode) {
		t.Fatalf("did not expect non-undefined-table code to match")
	}
}

func TestEnsureOutboxSchemaSQL_ContainsRequiredObjects(t *testing.T) {
	sql := outboxBootstrapSQL()
	required := []string{
		"CREATE TABLE IF NOT EXISTS outbox_events",
		"dedupe_key TEXT UNIQUE",
		"event_type TEXT NOT NULL",
		"status TEXT NOT NULL CHECK",
		"CREATE INDEX IF NOT EXISTS idx_outbox_pending_available",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected SQL to contain %q", fragment)
		}
	}
}
