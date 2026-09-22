package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

type Item struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	PriceCents int    `json:"price_cents"`
	Available  bool   `json:"available"`
}

var catalog = []Item{
	{ID: "burger-classic", Name: "Classic Burger", Category: "mains", PriceCents: 899, Available: true},
	{ID: "burger-double", Name: "Double Stack", Category: "mains", PriceCents: 1199, Available: true},
	{ID: "chicken-wrap", Name: "Chicken Wrap", Category: "mains", PriceCents: 799, Available: true},
	{ID: "fries-regular", Name: "Fries", Category: "sides", PriceCents: 349, Available: true},
	{ID: "onion-rings", Name: "Onion Rings", Category: "sides", PriceCents: 449, Available: false},
	{ID: "cola-large", Name: "Large Cola", Category: "drinks", PriceCents: 279, Available: true},
	{ID: "shake-vanilla", Name: "Vanilla Shake", Category: "drinks", PriceCents: 529, Available: true},
}

// Flipped false during shutdown so the readiness probe pulls this pod out of
// the Service before the listener stops accepting.
var ready atomic.Bool

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /menu", func(w http.ResponseWriter, r *http.Request) {
		items := catalog
		if category := r.URL.Query().Get("category"); category != "" {
			items = filterByCategory(category)
		}
		writeJSON(w, log, http.StatusOK, map[string]any{"items": items, "count": len(items)})
	})
	mux.HandleFunc("GET /menu/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		for _, item := range catalog {
			if item.ID == id {
				writeJSON(w, log, http.StatusOK, item)
				return
			}
		}
		writeJSON(w, log, http.StatusNotFound, map[string]string{"error": "item not found", "id": id})
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           requestLog(log, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		ready.Store(true)
		log.Info("listening", "port", port, "items", len(catalog))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	// Keep serving while kube-proxy removes this pod from the Service, otherwise
	// in-flight requests get connection-refused during a rolling update.
	ready.Store(false)
	log.Info("shutdown signal received, draining")
	time.Sleep(5 * time.Second)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
	log.Info("stopped")
}

func filterByCategory(category string) []Item {
	matched := make([]Item, 0, len(catalog))
	for _, item := range catalog {
		if item.Category == category {
			matched = append(matched, item)
		}
	}
	return matched
}

func writeJSON(w http.ResponseWriter, log *slog.Logger, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Error("encode response", "error", err)
	}
}

func requestLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		// Probes run every few seconds and would otherwise dominate the logs.
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			return
		}
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
