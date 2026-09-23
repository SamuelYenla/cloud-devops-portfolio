package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SamuelYenla/cloud-devops-portfolio/01-eks-ordering-platform/app/internal/httpx"
)

// stubs stands in for the menu and payments services. Testing against real
// HTTP servers rather than an interface keeps the client code under test,
// including status handling and JSON decoding.
type stubs struct {
	menu     *httptest.Server
	payments *httptest.Server
}

func (s *stubs) Close() {
	s.menu.Close()
	s.payments.Close()
}

func newStubs(t *testing.T, menu, payments http.HandlerFunc) *stubs {
	t.Helper()
	s := &stubs{
		menu:     httptest.NewServer(menu),
		payments: httptest.NewServer(payments),
	}
	t.Cleanup(s.Close)
	return s
}

func okMenu(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/menu/")
	items := map[string]menuItem{
		"burger-classic": {ID: "burger-classic", Name: "Classic Burger", PriceCents: 899, Available: true},
		"fries-regular":  {ID: "fries-regular", Name: "Fries", PriceCents: 349, Available: true},
		"onion-rings":    {ID: "onion-rings", Name: "Onion Rings", PriceCents: 449, Available: false},
	}
	item, ok := items[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(item)
}

func approvingPayments(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(payment{ID: "pay_0001", Status: "approved"})
}

func decliningPayments(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(payment{ID: "pay_0002", Status: "declined", DeclineCode: "amount_too_large"})
}

func handlerFor(t *testing.T, s *stubs) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	health := &httpx.Health{}
	health.SetReady(true)

	deps := &dependencies{
		menuURL:     s.menu.URL,
		paymentsURL: s.payments.URL,
		client:      &http.Client{Timeout: 2 * time.Second},
		log:         log,
	}
	return newHandler(log, health, deps, newStore())
}

func createOrder(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateOrderPricesFromMenuAndConfirms(t *testing.T) {
	h := handlerFor(t, newStubs(t, okMenu, approvingPayments))

	rec := createOrder(t, h, `{"items":[
		{"item_id":"burger-classic","quantity":2},
		{"item_id":"fries-regular","quantity":1}
	]}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	var order Order
	if err := json.NewDecoder(rec.Body).Decode(&order); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// 899*2 + 349 = 2147. The price comes from the menu service, never the
	// client, which is the whole reason orders calls out at all.
	const want = 899*2 + 349
	if order.TotalCents != want {
		t.Errorf("total = %d, want %d", order.TotalCents, want)
	}
	if order.Status != StatusConfirmed {
		t.Errorf("status = %q, want %q", order.Status, StatusConfirmed)
	}
	if order.PaymentID == "" {
		t.Error("confirmed order has no payment id")
	}
	if len(order.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(order.Lines))
	}
	if order.Lines[0].Name != "Classic Burger" {
		t.Errorf("line name = %q, want the name resolved from menu", order.Lines[0].Name)
	}
}

func TestDeclinedPaymentStillCreatesTheOrder(t *testing.T) {
	h := handlerFor(t, newStubs(t, okMenu, decliningPayments))

	rec := createOrder(t, h, `{"items":[{"item_id":"burger-classic","quantity":1}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var order Order
	if err := json.NewDecoder(rec.Body).Decode(&order); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if order.Status != StatusDeclined {
		t.Errorf("status = %q, want %q", order.Status, StatusDeclined)
	}
	if order.DeclineCode != "amount_too_large" {
		t.Errorf("decline_code = %q, want it passed through from payments", order.DeclineCode)
	}
}

func TestRequestErrorsAreDistinguishedFromDependencyErrors(t *testing.T) {
	tests := []struct {
		name     string
		menu     http.HandlerFunc
		payments http.HandlerFunc
		body     string
		want     int
	}{
		{
			name: "unknown item is the caller's fault",
			menu: okMenu, payments: approvingPayments,
			body: `{"items":[{"item_id":"no-such-item","quantity":1}]}`,
			want: http.StatusBadRequest,
		},
		{
			name: "unavailable item is a conflict",
			menu: okMenu, payments: approvingPayments,
			body: `{"items":[{"item_id":"onion-rings","quantity":1}]}`,
			want: http.StatusConflict,
		},
		{
			name:     "menu down is our fault, not the caller's",
			menu:     func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			payments: approvingPayments,
			body:     `{"items":[{"item_id":"burger-classic","quantity":1}]}`,
			want:     http.StatusBadGateway,
		},
		{
			name:     "payments down is our fault too",
			menu:     okMenu,
			payments: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			body:     `{"items":[{"item_id":"burger-classic","quantity":1}]}`,
			want:     http.StatusBadGateway,
		},
		{
			name: "empty order",
			menu: okMenu, payments: approvingPayments,
			body: `{"items":[]}`,
			want: http.StatusBadRequest,
		},
		{
			name: "zero quantity",
			menu: okMenu, payments: approvingPayments,
			body: `{"items":[{"item_id":"burger-classic","quantity":0}]}`,
			want: http.StatusBadRequest,
		},
		{
			name: "malformed json",
			menu: okMenu, payments: approvingPayments,
			body: `{"items":`,
			want: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := handlerFor(t, newStubs(t, tc.menu, tc.payments))
			rec := createOrder(t, h, tc.body)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d. body: %s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

// A dependency that accepts the connection and then never answers is worse
// than one that refuses it, because without a client timeout the request hangs
// until the caller gives up.
func TestHangingDependencyTimesOutRatherThanHanging(t *testing.T) {
	hang := func(w http.ResponseWriter, r *http.Request) { time.Sleep(3 * time.Second) }

	s := newStubs(t, hang, approvingPayments)
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	health := &httpx.Health{}
	health.SetReady(true)
	deps := &dependencies{
		menuURL:     s.menu.URL,
		paymentsURL: s.payments.URL,
		client:      &http.Client{Timeout: 200 * time.Millisecond},
		log:         log,
	}
	h := newHandler(log, health, deps, newStore())

	done := make(chan int, 1)
	go func() {
		done <- createOrder(t, h, `{"items":[{"item_id":"burger-classic","quantity":1}]}`).Code
	}()

	select {
	case code := <-done:
		if code != http.StatusBadGateway {
			t.Errorf("status = %d, want %d", code, http.StatusBadGateway)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request did not return: the client timeout is not being applied")
	}
}

func TestOrderIsRetrievableAndListed(t *testing.T) {
	h := handlerFor(t, newStubs(t, okMenu, approvingPayments))

	var created Order
	if err := json.NewDecoder(createOrder(t, h, `{"items":[{"item_id":"fries-regular","quantity":3}]}`).Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/orders/"+created.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get order = %d, want %d", rec.Code, http.StatusOK)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/orders", nil))
	var listed struct {
		Count  int     `json:"count"`
		Orders []Order `json:"orders"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if listed.Count != 1 {
		t.Errorf("count = %d, want 1", listed.Count)
	}
}

func TestUnknownOrderIsNotFound(t *testing.T) {
	h := handlerFor(t, newStubs(t, okMenu, approvingPayments))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/orders/ord_9999", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
