// Package httpx holds the HTTP plumbing every service in this repo shares:
// JSON responses, request logging, and a server lifecycle that drains before
// it exits.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// WriteJSON writes body as JSON. An encoding failure is logged rather than
// returned, since the status line has already gone out by then.
func WriteJSON(w http.ResponseWriter, log *slog.Logger, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Error("encode response", "error", err)
	}
}

// Error writes a JSON error body, so clients never have to parse two shapes.
func Error(w http.ResponseWriter, log *slog.Logger, status int, message string) {
	WriteJSON(w, log, status, map[string]string{"error": message})
}

// Logging records one line per request. Probe endpoints are skipped: they run
// every few seconds and would otherwise bury real traffic.
func Logging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", strconv.Itoa(rec.status),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
