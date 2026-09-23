package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/SamuelYenla/cloud-devops-portfolio/01-eks-ordering-platform/app/internal/httpx"
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

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	health := &httpx.Health{}
	if err := httpx.Serve(log, port, newHandler(log, health), health); err != nil {
		log.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func newHandler(log *slog.Logger, health *httpx.Health) http.Handler {
	mux := http.NewServeMux()
	health.Register(mux)

	mux.HandleFunc("GET /menu", func(w http.ResponseWriter, r *http.Request) {
		items := catalog
		if category := r.URL.Query().Get("category"); category != "" {
			items = filterByCategory(category)
		}
		httpx.WriteJSON(w, log, http.StatusOK, map[string]any{"items": items, "count": len(items)})
	})

	mux.HandleFunc("GET /menu/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if item, ok := lookup(id); ok {
			httpx.WriteJSON(w, log, http.StatusOK, item)
			return
		}
		httpx.WriteJSON(w, log, http.StatusNotFound, map[string]string{"error": "item not found", "id": id})
	})

	return httpx.Logging(log, mux)
}

func lookup(id string) (Item, bool) {
	for _, item := range catalog {
		if item.ID == id {
			return item, true
		}
	}
	return Item{}, false
}

func filterByCategory(category string) []Item {
	// Allocated rather than nil: encoding a nil slice yields JSON null, which
	// breaks clients that expect to range over an array.
	matched := make([]Item, 0, len(catalog))
	for _, item := range catalog {
		if item.Category == category {
			matched = append(matched, item)
		}
	}
	return matched
}
