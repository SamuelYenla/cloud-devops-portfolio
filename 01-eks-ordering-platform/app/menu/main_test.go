package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testHandler() http.Handler {
	return newHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	testHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestRouteStatus(t *testing.T) {
	ready.Store(true)

	tests := []struct {
		name   string
		method string
		target string
		want   int
	}{
		{"health", http.MethodGet, "/healthz", http.StatusOK},
		{"ready when serving", http.MethodGet, "/readyz", http.StatusOK},
		{"list", http.MethodGet, "/menu", http.StatusOK},
		{"known item", http.MethodGet, "/menu/fries-regular", http.StatusOK},
		{"unknown item", http.MethodGet, "/menu/does-not-exist", http.StatusNotFound},
		{"unknown path", http.MethodGet, "/nope", http.StatusNotFound},
		{"wrong method on list", http.MethodPost, "/menu", http.StatusMethodNotAllowed},
		{"wrong method on health", http.MethodDelete, "/healthz", http.StatusMethodNotAllowed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			testHandler().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.target, nil))
			if rec.Code != tc.want {
				t.Errorf("%s %s = %d, want %d", tc.method, tc.target, rec.Code, tc.want)
			}
		})
	}
}

func TestListReturnsWholeCatalog(t *testing.T) {
	ready.Store(true)
	rec := get(t, "/menu")

	var body struct {
		Count int    `json:"count"`
		Items []Item `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Count != len(catalog) {
		t.Errorf("count = %d, want %d", body.Count, len(catalog))
	}
	if len(body.Items) != len(catalog) {
		t.Errorf("items = %d, want %d", len(body.Items), len(catalog))
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func TestCategoryFilter(t *testing.T) {
	ready.Store(true)

	for _, category := range []string{"mains", "sides", "drinks"} {
		t.Run(category, func(t *testing.T) {
			rec := get(t, "/menu?category="+category)

			var body struct {
				Count int    `json:"count"`
				Items []Item `json:"items"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if body.Count == 0 {
				t.Fatalf("category %q returned nothing", category)
			}
			if body.Count != len(body.Items) {
				t.Errorf("count %d disagrees with %d items", body.Count, len(body.Items))
			}
			for _, item := range body.Items {
				if item.Category != category {
					t.Errorf("item %s has category %q, want %q", item.ID, item.Category, category)
				}
			}
		})
	}
}

func TestCategoryFilterUnknownIsEmptyNotNull(t *testing.T) {
	ready.Store(true)
	rec := get(t, "/menu?category=nonsense")

	// Encoding a nil slice yields JSON null, which breaks clients that expect to
	// range over an array, so filterByCategory must return an allocated slice.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(raw["items"]) != "[]" {
		t.Errorf("items = %s, want []", raw["items"])
	}
}

func TestGetItemReturnsMatchingItem(t *testing.T) {
	ready.Store(true)
	rec := get(t, "/menu/burger-double")

	var item Item
	if err := json.NewDecoder(rec.Body).Decode(&item); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if item.ID != "burger-double" {
		t.Errorf("id = %q, want burger-double", item.ID)
	}
	if item.PriceCents <= 0 {
		t.Errorf("price_cents = %d, want positive", item.PriceCents)
	}
}

// Readiness must fail while the process drains so the Service stops sending it
// traffic, but liveness must keep passing or the kubelet would restart the pod
// mid-shutdown.
func TestReadinessFailsWhileDrainingButLivenessHolds(t *testing.T) {
	ready.Store(false)
	t.Cleanup(func() { ready.Store(true) })

	if rec := get(t, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("/readyz = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if rec := get(t, "/healthz"); rec.Code != http.StatusOK {
		t.Errorf("/healthz = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec := get(t, "/menu"); rec.Code != http.StatusOK {
		t.Errorf("/menu = %d during drain, want %d", rec.Code, http.StatusOK)
	}
}

func TestCatalogIsInternallyConsistent(t *testing.T) {
	seen := make(map[string]bool, len(catalog))
	for _, item := range catalog {
		if seen[item.ID] {
			t.Errorf("duplicate id %q", item.ID)
		}
		seen[item.ID] = true

		if item.Name == "" {
			t.Errorf("item %q has no name", item.ID)
		}
		if item.Category == "" {
			t.Errorf("item %q has no category", item.ID)
		}
		if item.PriceCents <= 0 {
			t.Errorf("item %q has price_cents %d", item.ID, item.PriceCents)
		}
	}
}
