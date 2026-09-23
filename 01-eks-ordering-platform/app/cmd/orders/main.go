package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/SamuelYenla/cloud-devops-portfolio/01-eks-ordering-platform/app/internal/httpx"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	deps := &dependencies{
		menuURL:     envOr("MENU_URL", "http://menu.ordering.svc.cluster.local"),
		paymentsURL: envOr("PAYMENTS_URL", "http://payments-mock.ordering.svc.cluster.local"),
		// A client without a timeout waits forever on a hung dependency, ties
		// up a goroutine per request, and turns one slow service into an
		// outage in every service that calls it.
		client: &http.Client{Timeout: 3 * time.Second},
		log:    log,
	}

	log.Info("dependencies", "menu", deps.menuURL, "payments", deps.paymentsURL)

	health := &httpx.Health{}
	if err := httpx.Serve(log, port, newHandler(log, health, deps, newStore()), health); err != nil {
		log.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
