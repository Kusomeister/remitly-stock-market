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

func TestBuySuccessUpdatesBankWalletAndLog(t *testing.T) {
	handler := NewHandler()
	postStocks(t, handler, `{"stocks":[{"name":"stock1","quantity":2}]}`)

	rec := doRequest(handler, http.MethodPost, "/wallets/w1/stocks/stock1", `{"type":"buy"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	rec = doRequest(handler, http.MethodGet, "/stocks", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	assertStocksResponse(t, rec, []market.Stock{{Name: "stock1", Quantity: 1}})

	rec = doRequest(handler, http.MethodGet, "/wallets/w1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	assertWalletResponse(t, rec, market.Wallet{
		ID:     "w1",
		Stocks: []market.Stock{{Name: "stock1", Quantity: 1}},
	})

	rec = doRequest(handler, http.MethodGet, "/log", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	assertLogResponse(t, rec, []market.LogEntry{
		{Type: market.OperationBuy, WalletID: "w1", StockName: "stock1"},
	})
}

func TestSellSuccessUpdatesBankWalletAndLog(t *testing.T) {
	handler := NewHandler()
	postStocks(t, handler, `{"stocks":[{"name":"stock1","quantity":1}]}`)
	trade(t, handler, "/wallets/w1/stocks/stock1", `{"type":"buy"}`, http.StatusOK)

	rec := doRequest(handler, http.MethodPost, "/wallets/w1/stocks/stock1", `{"type":"sell"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	rec = doRequest(handler, http.MethodGet, "/stocks", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	assertStocksResponse(t, rec, []market.Stock{{Name: "stock1", Quantity: 1}})

	rec = doRequest(handler, http.MethodGet, "/wallets/w1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	assertWalletResponse(t, rec, market.Wallet{ID: "w1", Stocks: []market.Stock{}})

	rec = doRequest(handler, http.MethodGet, "/log", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	assertLogResponse(t, rec, []market.LogEntry{
		{Type: market.OperationBuy, WalletID: "w1", StockName: "stock1"},
		{Type: market.OperationSell, WalletID: "w1", StockName: "stock1"},
	})
}

func TestAuditLogPreservesBuySellOrder(t *testing.T) {
	handler := NewHandler()
	postStocks(t, handler, `{"stocks":[{"name":"stock1","quantity":2}]}`)
	trade(t, handler, "/wallets/w1/stocks/stock1", `{"type":"buy"}`, http.StatusOK)
	trade(t, handler, "/wallets/w1/stocks/stock1", `{"type":"sell"}`, http.StatusOK)
	trade(t, handler, "/wallets/w2/stocks/stock1", `{"type":"buy"}`, http.StatusOK)

	rec := doRequest(handler, http.MethodGet, "/log", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	assertLogResponse(t, rec, []market.LogEntry{
		{Type: market.OperationBuy, WalletID: "w1", StockName: "stock1"},
		{Type: market.OperationSell, WalletID: "w1", StockName: "stock1"},
		{Type: market.OperationBuy, WalletID: "w2", StockName: "stock1"},
	})
}

func TestBuyUnknownStockReturnsNotFound(t *testing.T) {
	handler := NewHandler()
	postStocks(t, handler, `{"stocks":[]}`)

	trade(t, handler, "/wallets/w1/stocks/missing", `{"type":"buy"}`, http.StatusNotFound)
}

func TestBuyStockWithZeroQuantityReturnsBadRequest(t *testing.T) {
	handler := NewHandler()
	postStocks(t, handler, `{"stocks":[{"name":"stock1","quantity":0}]}`)

	trade(t, handler, "/wallets/w1/stocks/stock1", `{"type":"buy"}`, http.StatusBadRequest)
}

func TestSellUnknownStockReturnsNotFound(t *testing.T) {
	handler := NewHandler()
	postStocks(t, handler, `{"stocks":[]}`)

	trade(t, handler, "/wallets/w1/stocks/missing", `{"type":"sell"}`, http.StatusNotFound)
}

func TestSellStockMissingFromWalletReturnsBadRequest(t *testing.T) {
	handler := NewHandler()
	postStocks(t, handler, `{"stocks":[{"name":"stock1","quantity":1}]}`)

	trade(t, handler, "/wallets/w1/stocks/stock1", `{"type":"sell"}`, http.StatusBadRequest)
}

func TestInvalidOperationTypeReturnsBadRequest(t *testing.T) {
	handler := NewHandler()
	postStocks(t, handler, `{"stocks":[{"name":"stock1","quantity":1}]}`)

	trade(t, handler, "/wallets/w1/stocks/stock1", `{"type":"hold"}`, http.StatusBadRequest)
}

func TestInvalidTradeJSONReturnsBadRequest(t *testing.T) {
	handler := NewHandler()
	postStocks(t, handler, `{"stocks":[{"name":"stock1","quantity":1}]}`)

	trade(t, handler, "/wallets/w1/stocks/stock1", `{"type":`, http.StatusBadRequest)
}

func TestFailedOperationDoesNotAddAuditLogEntry(t *testing.T) {
	handler := NewHandler()
	postStocks(t, handler, `{"stocks":[{"name":"stock1","quantity":0}]}`)
	trade(t, handler, "/wallets/w1/stocks/stock1", `{"type":"buy"}`, http.StatusBadRequest)

	rec := doRequest(handler, http.MethodGet, "/log", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	assertLogResponse(t, rec, []market.LogEntry{})
}

func TestPostStocksDoesNotClearWalletsOrAuditLog(t *testing.T) {
	handler := NewHandler()
	postStocks(t, handler, `{"stocks":[{"name":"stock1","quantity":1}]}`)
	trade(t, handler, "/wallets/w1/stocks/stock1", `{"type":"buy"}`, http.StatusOK)
	postStocks(t, handler, `{"stocks":[]}`)

	rec := doRequest(handler, http.MethodGet, "/wallets/w1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	assertWalletResponse(t, rec, market.Wallet{
		ID:     "w1",
		Stocks: []market.Stock{{Name: "stock1", Quantity: 1}},
	})

	rec = doRequest(handler, http.MethodGet, "/log", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	assertLogResponse(t, rec, []market.LogEntry{
		{Type: market.OperationBuy, WalletID: "w1", StockName: "stock1"},
	})
}

func TestGetMissingWalletReturnsEmptyWallet(t *testing.T) {
	rec := doRequest(NewHandler(), http.MethodGet, "/wallets/w1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	assertWalletResponse(t, rec, market.Wallet{ID: "w1", Stocks: []market.Stock{}})
}

func TestGetMissingWalletStockReturnsZero(t *testing.T) {
	handler := NewHandler()

	rec := doRequest(handler, http.MethodGet, "/wallets/w1/stocks/stock1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	assertNumberResponse(t, rec, 0)
}

func TestGetMissingStockInExistingWalletReturnsZero(t *testing.T) {
	handler := NewHandler()
	postStocks(t, handler, `{"stocks":[{"name":"stock1","quantity":1},{"name":"stock2","quantity":1}]}`)
	trade(t, handler, "/wallets/w1/stocks/stock1", `{"type":"buy"}`, http.StatusOK)

	rec := doRequest(handler, http.MethodGet, "/wallets/w1/stocks/stock2", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	assertNumberResponse(t, rec, 0)
}

func TestInvalidWalletPathsReturnNotFound(t *testing.T) {
	tests := []string{
		"/wallets/",
		"/wallets/w1/stocks",
		"/wallets/w1/invalid/stock1",
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			rec := doRequest(NewHandler(), http.MethodGet, target, "")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
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

func postStocks(t *testing.T, handler http.Handler, body string) {
	t.Helper()

	rec := doRequest(handler, http.MethodPost, "/stocks", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func trade(t *testing.T, handler http.Handler, target, body string, expectedStatus int) {
	t.Helper()

	rec := doRequest(handler, http.MethodPost, target, body)
	if rec.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d", expectedStatus, rec.Code)
	}
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

func assertWalletResponse(t *testing.T, rec *httptest.ResponseRecorder, expected market.Wallet) {
	t.Helper()

	var body market.Wallet
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON body: %v", err)
	}

	if !reflect.DeepEqual(body, expected) {
		t.Fatalf("expected wallet %#v, got %#v", expected, body)
	}
}

func assertLogResponse(t *testing.T, rec *httptest.ResponseRecorder, expected []market.LogEntry) {
	t.Helper()

	var body logResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON body: %v", err)
	}

	if !reflect.DeepEqual(body.Log, expected) {
		t.Fatalf("expected log %#v, got %#v", expected, body.Log)
	}
}

func assertNumberResponse(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()

	var body int
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON body: %v", err)
	}

	if body != expected {
		t.Fatalf("expected number %d, got %d", expected, body)
	}
}
