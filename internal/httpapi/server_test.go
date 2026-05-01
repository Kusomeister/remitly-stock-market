package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"remitly-stock-market/internal/market"
)

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	NewHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON body: %v", err)
	}

	if body.Status != "ok" {
		t.Fatalf("expected status ok, got %q", body.Status)
	}
}

func TestGetStocksInitiallyEmpty(t *testing.T) {
	rec := doRequest(NewHandler(), http.MethodGet, "/stocks", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	assertStocksResponse(t, rec, []market.Stock{})
}

func TestPostStocksSetsState(t *testing.T) {
	handler := NewHandler()
	postBody := `{"stocks":[{"name":"stock1","quantity":99},{"name":"stock2","quantity":1}]}`

	rec := doRequest(handler, http.MethodPost, "/stocks", postBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	rec = doRequest(handler, http.MethodGet, "/stocks", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	assertStocksResponse(t, rec, []market.Stock{
		{Name: "stock1", Quantity: 99},
		{Name: "stock2", Quantity: 1},
	})
}

func TestPostStocksEmptyListClearsBank(t *testing.T) {
	handler := NewHandler()

	rec := doRequest(handler, http.MethodPost, "/stocks", `{"stocks":[{"name":"stock1","quantity":99}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	rec = doRequest(handler, http.MethodPost, "/stocks", `{"stocks":[]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	rec = doRequest(handler, http.MethodGet, "/stocks", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	assertStocksResponse(t, rec, []market.Stock{})
}

func TestPostStocksValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `{"stocks":[`},
		{name: "missing stocks", body: `{}`},
		{name: "null stocks", body: `{"stocks":null}`},
		{name: "negative quantity", body: `{"stocks":[{"name":"stock1","quantity":-1}]}`},
		{name: "empty stock name", body: `{"stocks":[{"name":"","quantity":1}]}`},
		{name: "duplicate stock name", body: `{"stocks":[{"name":"stock1","quantity":1},{"name":"stock1","quantity":2}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(NewHandler(), http.MethodPost, "/stocks", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
			}
		})
	}
}

func doRequest(handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	return rec
}

func assertStocksResponse(t *testing.T, rec *httptest.ResponseRecorder, expected []market.Stock) {
	t.Helper()

	var body stocksResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON body: %v", err)
	}

	if !reflect.DeepEqual(body.Stocks, expected) {
		t.Fatalf("expected stocks %#v, got %#v", expected, body.Stocks)
	}
}
