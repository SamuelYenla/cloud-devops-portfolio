package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

// DrainDelay is how long a service keeps serving after SIGTERM before it starts
// shutting down. Kubernetes removes a pod from Service endpoints asynchronously,
// so a process that exits immediately refuses connections kube-proxy is still
// routing to it. Pod specs must set terminationGracePeriodSeconds above this
// plus ShutdownTimeout.
const (
	DrainDelay      = 5 * time.Second
	ShutdownTimeout = 10 * time.Second
)

// Health carries readiness between the probe handlers and the shutdown path.
// The zero value is not ready, which is what we want before the listener is up.
type Health struct {
	ready atomic.Bool
}

func (h *Health) SetReady(v bool) { h.ready.Store(v) }
func (h *Health) IsReady() bool   { return h.ready.Load() }

// Register wires liveness and readiness.
//
// Liveness stays green during a drain on purpose: reporting unhealthy there
// would have the kubelet restart a pod that is deliberately shutting down.
func (h *Health) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !h.IsReady() {
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

// Serve runs handler until SIGTERM or SIGINT, then drains and shuts down.
// It marks health ready once the listener is accepting.
func Serve(log *slog.Logger, port string, handler http.Handler, health *Health) error {
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		health.SetReady(true)
		log.Info("listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}

	health.SetReady(false)
	log.Info("shutdown signal received, draining", "drain", DrainDelay.String())
	time.Sleep(DrainDelay)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	log.Info("stopped")
	return nil
}
