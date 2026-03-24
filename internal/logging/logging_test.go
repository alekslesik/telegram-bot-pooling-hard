package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewWithWriter_logLevelNonDebug(t *testing.T) {
	t.Setenv("LOG_LEVEL", "info")
	t.Cleanup(func() { _ = os.Unsetenv("LOG_LEVEL") })

	var buf bytes.Buffer
	log := NewWithWriter(&buf)
	log.Debug("hidden")
	log.Info("shown")
	if strings.Contains(buf.String(), "hidden") {
		t.Fatalf("debug should be filtered: %s", buf.String())
	}
}

func TestNewWithWriter_timezoneLoadFailsUsesLocal(t *testing.T) {
	orig := loadTZEuropeMoscow
	t.Cleanup(func() { loadTZEuropeMoscow = orig })
	loadTZEuropeMoscow = func() (*time.Location, error) {
		return nil, errors.New("tz")
	}

	var buf bytes.Buffer
	log := NewWithWriter(&buf)
	log.Info("ok")
	if !strings.Contains(buf.String(), "ok") {
		t.Fatalf("expected log line: %s", buf.String())
	}
}

func TestNewWithWriter_defaultText(t *testing.T) {
	t.Cleanup(func() {
		_ = os.Unsetenv("LOG_LEVEL")
		_ = os.Unsetenv("LOG_FORMAT")
	})
	_ = os.Unsetenv("LOG_LEVEL")
	_ = os.Unsetenv("LOG_FORMAT")

	var buf bytes.Buffer
	log := NewWithWriter(&buf)
	log.Info("hello", "k", "v")

	out := buf.String()
	if !strings.Contains(out, "hello") || !strings.Contains(out, "k") {
		t.Fatalf("unexpected log output: %q", out)
	}
}

func TestNewWithWriter_debugJSON(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "json")

	var buf bytes.Buffer
	log := NewWithWriter(&buf)
	log.Debug("dbg")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("json log: %v", err)
	}
	if m["msg"] != "dbg" {
		t.Fatalf("expected msg dbg, got %v", m["msg"])
	}
}

func TestNewWithWriter_timeReplaceUsesMoscowOrLocal(t *testing.T) {
	t.Cleanup(func() {
		_ = os.Unsetenv("LOG_FORMAT")
	})
	_ = os.Unsetenv("LOG_FORMAT")

	var buf bytes.Buffer
	log := NewWithWriter(&buf)
	log.Info("t")

	out := buf.String()
	if !strings.Contains(out, "time=") || !strings.Contains(out, "/") {
		t.Fatalf("expected slog time field with DD/MM/YYYY layout, got %q", out)
	}
}

func TestNewFromEnv_delegates(t *testing.T) {
	t.Cleanup(func() {
		_ = os.Unsetenv("LOG_FORMAT")
	})
	_ = os.Unsetenv("LOG_FORMAT")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	log := NewFromEnv()
	log.Info("from env")
	_ = w.Close()

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	if !strings.Contains(buf.String(), "from env") {
		t.Fatalf("expected log on stdout pipe, got %q", buf.String())
	}
}
