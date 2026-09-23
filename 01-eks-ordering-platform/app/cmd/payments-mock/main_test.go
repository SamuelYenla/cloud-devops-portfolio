package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/SamuelYenla/cloud-devops-portfolio/01-eks-ordering-platform/app/internal/httpx"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	health := &httpx.Health{}
	health.SetReady(true)
	return newHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), health, newStore())
}

func post(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/payments", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthorizeApproves(t *testing.T) {
	rec := post(t, testHandler(t), `{"order_id":"ord_0001","amount_cents":1899}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var p Payment
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Status != StatusApproved {
		t.Errorf("status = %q, want %q", p.Status, StatusApproved)
	}
	if p.ID == "" {
		t.Error("payment id is empty")
	}
	if p.Currency != "USD" {
		t.Errorf("currency = %q, want USD as the default", p.Currency)
	}
}

// A declined authorization is a successful call with a negative result, so it
// must not be reported as an HTTP error.
func TestLargeAmountIsDeclinedButStillCreated(t *testing.T) {
	rec := post(t, testHandler(t), `{"order_id":"ord_0002","amount_cents":50000}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var p Payment
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Status != StatusDeclined {
		t.Errorf("status = %q, want %q", p.Status, StatusDeclined)
	}
	if p.DeclineCode == "" {
		t.Error("a declined payment should carry a decline_code")
	}
}

func TestDeclineThresholdBoundary(t *testing.T) {
	tests := []struct {
		amount int
		want   string
	}{
		{declineThresholdCents - 1, StatusApproved},
		{declineThresholdCents, StatusDeclined},
		{declineThresholdCents + 1, StatusDeclined},
	}

	for _, tc := range tests {
		rec := post(t, testHandler(t), `{"order_id":"ord","amount_cents":`+strconv.Itoa(tc.amount)+`}`)
		var p Payment
		if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if p.Status != tc.want {
			t.Errorf("amount %d = %q, want %q", tc.amount, p.Status, tc.want)
		}
	}
}

func TestRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"malformed json", `{"order_id":`},
		{"missing order id", `{"amount_cents":100}`},
		{"zero amount", `{"order_id":"ord","amount_cents":0}`},
		{"negative amount", `{"order_id":"ord","amount_cents":-500}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := post(t, testHandler(t), tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestPaymentIsRetrievableAfterAuthorizing(t *testing.T) {
	h := testHandler(t)

	var created Payment
	if err := json.NewDecoder(post(t, h, `{"order_id":"ord_0003","amount_cents":750}`).Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/payments/"+created.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var fetched Payment
	if err := json.NewDecoder(rec.Body).Decode(&fetched); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if fetched.ID != created.ID || fetched.AmountCents != created.AmountCents {
		t.Errorf("fetched %+v does not match created %+v", fetched, created)
	}
}

func TestUnknownPaymentIsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	testHandler(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/payments/pay_9999", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestPaymentIDsAreUnique(t *testing.T) {
	h := testHandler(t)
	seen := make(map[string]bool)

	for i := 0; i < 25; i++ {
		var p Payment
		if err := json.NewDecoder(post(t, h, `{"order_id":"ord","amount_cents":100}`).Body).Decode(&p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if seen[p.ID] {
			t.Fatalf("duplicate payment id %q", p.ID)
		}
		seen[p.ID] = true
	}
}
