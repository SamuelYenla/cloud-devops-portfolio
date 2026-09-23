package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/SamuelYenla/cloud-devops-portfolio/01-eks-ordering-platform/app/internal/httpx"
)

type AuthorizeRequest struct {
	OrderID     string `json:"order_id"`
	AmountCents int    `json:"amount_cents"`
	Currency    string `json:"currency"`
}

type Payment struct {
	ID           string    `json:"id"`
	OrderID      string    `json:"order_id"`
	AmountCents  int       `json:"amount_cents"`
	Currency     string    `json:"currency"`
	Status       string    `json:"status"`
	DeclineCode  string    `json:"decline_code,omitempty"`
	AuthorizedAt time.Time `json:"authorized_at"`
}

const (
	StatusApproved = "approved"
	StatusDeclined = "declined"

	// Anything at or above this declines. A fixed rule rather than a random
	// one keeps tests and demos reproducible — a mock that fails randomly is
	// impossible to write assertions against.
	declineThresholdCents = 50_000
)

type store struct {
	mu       sync.RWMutex
	payments map[string]Payment
	seq      int
}

func newStore() *store {
	return &store{payments: make(map[string]Payment)}
}

func (s *store) save(p Payment) Payment {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	p.ID = fmt.Sprintf("pay_%04d", s.seq)
	s.payments[p.ID] = p
	return p
}

func (s *store) get(id string) (Payment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.payments[id]
	return p, ok
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	health := &httpx.Health{}
	if err := httpx.Serve(log, port, newHandler(log, health, newStore()), health); err != nil {
		log.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func newHandler(log *slog.Logger, health *httpx.Health, st *store) http.Handler {
	mux := http.NewServeMux()
	health.Register(mux)

	mux.HandleFunc("POST /payments", func(w http.ResponseWriter, r *http.Request) {
		var req AuthorizeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.Error(w, log, http.StatusBadRequest, "malformed JSON body")
			return
		}

		switch {
		case req.OrderID == "":
			httpx.Error(w, log, http.StatusBadRequest, "order_id is required")
			return
		case req.AmountCents <= 0:
			httpx.Error(w, log, http.StatusBadRequest, "amount_cents must be positive")
			return
		}

		if req.Currency == "" {
			req.Currency = "USD"
		}

		payment := Payment{
			OrderID:      req.OrderID,
			AmountCents:  req.AmountCents,
			Currency:     req.Currency,
			Status:       StatusApproved,
			AuthorizedAt: time.Now().UTC(),
		}
		if req.AmountCents >= declineThresholdCents {
			payment.Status = StatusDeclined
			payment.DeclineCode = "amount_too_large"
		}

		saved := st.save(payment)

		// A declined authorization is a successful API call that returns a
		// negative result, so it is 201 rather than an error status. The
		// caller reads Status, not the HTTP code.
		httpx.WriteJSON(w, log, http.StatusCreated, saved)
	})

	mux.HandleFunc("GET /payments/{id}", func(w http.ResponseWriter, r *http.Request) {
		if p, ok := st.get(r.PathValue("id")); ok {
			httpx.WriteJSON(w, log, http.StatusOK, p)
			return
		}
		httpx.Error(w, log, http.StatusNotFound, "payment not found")
	})

	return httpx.Logging(log, mux)
}
