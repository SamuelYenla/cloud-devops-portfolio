package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

// errItemNotFound separates "the menu answered, and this item does not exist"
// from "the menu did not answer". The first is the caller's fault, the second
// is ours, and they map to different status codes.
var errItemNotFound = errors.New("menu item not found")

type menuItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PriceCents int    `json:"price_cents"`
	Available  bool   `json:"available"`
}

type payment struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	DeclineCode string `json:"decline_code,omitempty"`
}

type dependencies struct {
	menuURL     string
	paymentsURL string
	client      *http.Client
	log         *slog.Logger
}

func (d *dependencies) lookupItem(ctx context.Context, id string) (menuItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.menuURL+"/menu/"+id, nil)
	if err != nil {
		return menuItem{}, err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return menuItem{}, fmt.Errorf("calling menu: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return menuItem{}, errItemNotFound
	default:
		return menuItem{}, fmt.Errorf("menu returned %d", resp.StatusCode)
	}

	var item menuItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return menuItem{}, fmt.Errorf("decoding menu response: %w", err)
	}
	return item, nil
}

func (d *dependencies) authorize(ctx context.Context, orderID string, amountCents int) (payment, error) {
	body, err := json.Marshal(map[string]any{
		"order_id":     orderID,
		"amount_cents": amountCents,
		"currency":     "USD",
	})
	if err != nil {
		return payment{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.paymentsURL+"/payments", bytes.NewReader(body))
	if err != nil {
		return payment{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return payment{}, fmt.Errorf("calling payments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return payment{}, fmt.Errorf("payments returned %d", resp.StatusCode)
	}

	var p payment
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return payment{}, fmt.Errorf("decoding payments response: %w", err)
	}
	return p, nil
}
