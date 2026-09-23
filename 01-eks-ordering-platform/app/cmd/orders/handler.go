package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/SamuelYenla/cloud-devops-portfolio/01-eks-ordering-platform/app/internal/httpx"
)

type LineRequest struct {
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
}

type CreateOrderRequest struct {
	Items []LineRequest `json:"items"`
}

type Line struct {
	ItemID     string `json:"item_id"`
	Name       string `json:"name"`
	Quantity   int    `json:"quantity"`
	PriceCents int    `json:"price_cents"`
	TotalCents int    `json:"total_cents"`
}

type Order struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Lines       []Line    `json:"lines"`
	TotalCents  int       `json:"total_cents"`
	PaymentID   string    `json:"payment_id,omitempty"`
	DeclineCode string    `json:"decline_code,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

const (
	StatusConfirmed = "confirmed"
	StatusDeclined  = "payment_declined"

	maxLinesPerOrder = 50
)

type store struct {
	mu     sync.RWMutex
	orders map[string]Order
	seq    int
}

func newStore() *store {
	return &store{orders: make(map[string]Order)}
}

func (s *store) nextID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	return fmt.Sprintf("ord_%04d", s.seq)
}

func (s *store) save(o Order) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orders[o.ID] = o
}

func (s *store) get(id string) (Order, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o, ok := s.orders[id]
	return o, ok
}

func (s *store) list() []Order {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make([]Order, 0, len(s.orders))
	for _, o := range s.orders {
		all = append(all, o)
	}
	return all
}

func newHandler(log *slog.Logger, health *httpx.Health, deps *dependencies, st *store) http.Handler {
	mux := http.NewServeMux()
	health.Register(mux)

	mux.HandleFunc("POST /orders", func(w http.ResponseWriter, r *http.Request) {
		var req CreateOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.Error(w, log, http.StatusBadRequest, "malformed JSON body")
			return
		}
		switch {
		case len(req.Items) == 0:
			httpx.Error(w, log, http.StatusBadRequest, "at least one item is required")
			return
		case len(req.Items) > maxLinesPerOrder:
			httpx.Error(w, log, http.StatusBadRequest, "too many line items")
			return
		}

		lines := make([]Line, 0, len(req.Items))
		total := 0

		for _, requested := range req.Items {
			if requested.Quantity <= 0 {
				httpx.Error(w, log, http.StatusBadRequest,
					fmt.Sprintf("quantity for %q must be positive", requested.ItemID))
				return
			}

			item, err := deps.lookupItem(r.Context(), requested.ItemID)
			switch {
			case errors.Is(err, errItemNotFound):
				httpx.Error(w, log, http.StatusBadRequest,
					fmt.Sprintf("unknown item %q", requested.ItemID))
				return
			case err != nil:
				// The menu service is the one that failed, not the caller, so
				// this is 502 rather than 400 or 500.
				log.Error("menu lookup failed", "item", requested.ItemID, "error", err)
				httpx.Error(w, log, http.StatusBadGateway, "menu service unavailable")
				return
			}

			if !item.Available {
				httpx.Error(w, log, http.StatusConflict,
					fmt.Sprintf("%q is not available", item.ID))
				return
			}

			lineTotal := item.PriceCents * requested.Quantity
			total += lineTotal
			lines = append(lines, Line{
				ItemID:     item.ID,
				Name:       item.Name,
				Quantity:   requested.Quantity,
				PriceCents: item.PriceCents,
				TotalCents: lineTotal,
			})
		}

		order := Order{
			ID:         st.nextID(),
			Lines:      lines,
			TotalCents: total,
			CreatedAt:  time.Now().UTC(),
		}

		auth, err := deps.authorize(r.Context(), order.ID, total)
		if err != nil {
			log.Error("authorization failed", "order", order.ID, "error", err)
			httpx.Error(w, log, http.StatusBadGateway, "payment service unavailable")
			return
		}

		order.PaymentID = auth.ID
		if auth.Status == "approved" {
			order.Status = StatusConfirmed
		} else {
			order.Status = StatusDeclined
			order.DeclineCode = auth.DeclineCode
		}
		st.save(order)

		// A declined payment still produced an order record, so this is 201
		// with a status the caller reads, not an HTTP error.
		httpx.WriteJSON(w, log, http.StatusCreated, order)
	})

	mux.HandleFunc("GET /orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		if order, ok := st.get(r.PathValue("id")); ok {
			httpx.WriteJSON(w, log, http.StatusOK, order)
			return
		}
		httpx.Error(w, log, http.StatusNotFound, "order not found")
	})

	mux.HandleFunc("GET /orders", func(w http.ResponseWriter, r *http.Request) {
		orders := st.list()
		httpx.WriteJSON(w, log, http.StatusOK, map[string]any{"orders": orders, "count": len(orders)})
	})

	return httpx.Logging(log, mux)
}
