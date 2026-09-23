package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SamuelYenla/cloud-devops-portfolio/01-eks-ordering-platform/app/internal/httpx"
)

func testHandler(t *testing.T) (http.Handler, *httpx.Health) {
	t.Helper()
	health := &httpx.Health{}
	health.SetReady(true)
	return newHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), health), health
}

func get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	h, _ := testHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestRouteStatus(t *testing.T) {
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := testHandler(t)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.target, nil))
			if rec.Code != tc.want {
				t.Errorf("%s %s = %d, want %d", tc.method, tc.target, rec.Code, tc.want)
			}
		})
	}
}

func TestListReturnsWholeCatalog(t *testing.T) {
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
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func TestCategoryFilter(t *testing.T) {
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
			for _, item := range body.Items {
				if item.Category != category {
					t.Errorf("item %s has category %q, want %q", item.ID, item.Category, category)
				}
			}
		})
	}
}

func TestUnknownCategoryIsEmptyArrayNotNull(t *testing.T) {
	rec := get(t, "/menu?category=nonsense")

	// A nil slice encodes as JSON null, which breaks clients that range over
	// the result.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(raw["items"]) != "[]" {
		t.Errorf("items = %s, want []", raw["items"])
	}
}

func TestReadinessFailsWhileDrainingButLivenessHolds(t *testing.T) {
	h, health := testHandler(t)
	health.SetReady(false)

	check := func(path string, want int) {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("%s = %d, want %d", path, rec.Code, want)
		}
	}

	check("/readyz", http.StatusServiceUnavailable)
	check("/healthz", http.StatusOK)
	check("/menu", http.StatusOK)
}

func TestCatalogIsInternallyConsistent(t *testing.T) {
	seen := make(map[string]bool, len(catalog))
	for _, item := range catalog {
		if seen[item.ID] {
			t.Errorf("duplicate id %q", item.ID)
		}
		seen[item.ID] = true

		if item.Name == "" || item.Category == "" {
			t.Errorf("item %q is missing a name or category", item.ID)
		}
		if item.PriceCents <= 0 {
			t.Errorf("item %q has price_cents %d", item.ID, item.PriceCents)
		}
	}
}
