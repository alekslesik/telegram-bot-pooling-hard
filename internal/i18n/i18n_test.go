package i18n

import "testing"

func TestResolve(t *testing.T) {
	if got := Resolve("en", "ru"); got != En {
		t.Fatalf("stored en: got %v", got)
	}
	if got := Resolve("", "en-US"); got != En {
		t.Fatalf("telegram en: got %v", got)
	}
	if got := Resolve("", "de"); got != Ru {
		t.Fatalf("default ru: got %v", got)
	}
}
