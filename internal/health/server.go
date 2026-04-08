package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/alekslesik/telegram-bot-pooling-hard/internal/cache"
)

type Server struct {
	httpServer *http.Server
}

type checkResult struct {
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type readinessResponse struct {
	Status string                 `json:"status"`
	Checks map[string]checkResult `json:"checks"`
}

type healthResponse struct {
	Status string `json:"status"`
}

func NewServer(addr string, db *sql.DB, redisClient *cache.Redis, outboxEnabled bool) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/readyz", readinessHandler(db, redisClient, outboxEnabled))
	return &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 3 * time.Second,
		},
	}
}

func (s *Server) Start() error {
	if s == nil || s.httpServer == nil {
		return nil
	}
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func readinessHandler(db *sql.DB, redisClient *cache.Redis, outboxEnabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		checks := map[string]checkResult{
			"outbox": {OK: true, Detail: boolDetail(outboxEnabled, "enabled", "disabled")},
		}
		ready := true

		if db != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := db.PingContext(ctx)
			cancel()
			if err != nil {
				ready = false
				checks["database"] = checkResult{OK: false, Detail: "ping failed"}
			} else {
				checks["database"] = checkResult{OK: true, Detail: "postgres"}
			}
		} else {
			checks["database"] = checkResult{OK: true, Detail: "memory"}
		}

		if redisClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := redisClient.Ping(ctx)
			cancel()
			if err != nil {
				ready = false
				checks["redis"] = checkResult{OK: false, Detail: "ping failed"}
			} else {
				checks["redis"] = checkResult{OK: true, Detail: "ping"}
			}
		} else {
			checks["redis"] = checkResult{OK: true, Detail: "disabled"}
		}

		status := "ready"
		httpCode := http.StatusOK
		if !ready {
			status = "not_ready"
			httpCode = http.StatusServiceUnavailable
		}
		writeJSON(w, httpCode, readinessResponse{Status: status, Checks: checks})
	}
}

func boolDetail(v bool, yes, no string) string {
	if v {
		return yes
	}
	return no
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
