package dbconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDSN_FromDB_DSN(t *testing.T) {
	t.Setenv("DB_DSN", "postgres://a:b@h:5432/d?sslmode=disable")
	t.Setenv("DB_PASSWORD_FILE", "")
	t.Cleanup(func() {
		_ = os.Unsetenv("DB_DSN")
	})
	got, err := ResolveDSN()
	if err != nil {
		t.Fatal(err)
	}
	if got != "postgres://a:b@h:5432/d?sslmode=disable" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveDSN_FromPasswordFile(t *testing.T) {
	t.Setenv("DB_DSN", "")
	t.Setenv("DB_USER", "app")
	t.Setenv("DB_HOST", "postgres")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "mydb")

	dir := t.TempDir()
	p := filepath.Join(dir, "pw")
	if err := os.WriteFile(p, []byte("s3cr3t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DB_PASSWORD_FILE", p)

	got, err := ResolveDSN()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "postgres://app:") || !strings.Contains(got, "@postgres:5432/mydb") {
		t.Fatalf("unexpected dsn: %q", got)
	}
	if !strings.Contains(got, "sslmode=disable") {
		t.Fatalf("missing sslmode: %q", got)
	}
}

func TestResolveDSN_Empty(t *testing.T) {
	t.Setenv("DB_DSN", "")
	t.Setenv("DB_PASSWORD_FILE", "")
	got, err := ResolveDSN()
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}
