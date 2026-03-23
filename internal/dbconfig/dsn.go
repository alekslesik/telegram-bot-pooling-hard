package dbconfig

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// ResolveDSN returns a lib/pq connection string.
// Precedence: DB_DSN if set; else build from DB_PASSWORD_FILE + DB_HOST, DB_PORT, DB_NAME, DB_USER.
// If nothing is configured, returns ("", nil) for in-memory mode.
func ResolveDSN() (string, error) {
	dsn := strings.TrimSpace(os.Getenv("DB_DSN"))
	if dsn != "" {
		return dsn, nil
	}
	path := strings.TrimSpace(os.Getenv("DB_PASSWORD_FILE"))
	if path == "" {
		return "", nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read DB_PASSWORD_FILE: %w", err)
	}
	password := strings.TrimSpace(strings.TrimSuffix(string(raw), "\n"))
	if password == "" {
		return "", fmt.Errorf("DB_PASSWORD_FILE is empty")
	}
	user := getenvDefault("DB_USER", "postgres")
	host := getenvDefault("DB_HOST", "localhost")
	port := getenvDefault("DB_PORT", "5432")
	dbname := strings.TrimPrefix(strings.TrimSpace(getenvDefault("DB_NAME", "postgres")), "/")

	u := url.UserPassword(user, password)
	return (&url.URL{
		Scheme:   "postgres",
		User:     u,
		Host:     net.JoinHostPort(host, port),
		Path:     "/" + dbname,
		RawQuery: "sslmode=disable",
	}).String(), nil
}

func getenvDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
