package market

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestMemoryMarketInitialState(t *testing.T) {
	ctx := context.Background()
	market := NewMemoryMarket()

	stocks, err := market.ListStocks(ctx)
	if err != nil {
		t.Fatalf("list stocks: %v", err)
	}
	assertStocks(t, stocks, []Stock{})

	wallet, err := market.GetWallet(ctx, "w1")
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	assertWallet(t, wallet, Wallet{ID: "w1", Stocks: []Stock{}})

	quantity, err := market.GetWalletStock(ctx, "w1", "stock1")
	if err != nil {
		t.Fatalf("get wallet stock: %v", err)
	}
	if quantity != 0 {
		t.Fatalf("expected missing wallet stock quantity 0, got %d", quantity)
	}

	logEntries, err := market.ListLog(ctx)
	if err != nil {
		t.Fatalf("list log: %v", err)
	}
	assertLog(t, logEntries, []LogEntry{})
}

func TestMemoryMarketSetStocksSetsAndClearsBank(t *testing.T) {
	ctx := context.Background()
	market := NewMemoryMarket()

	err := market.SetStocks(ctx, []Stock{
		{Name: "stock1", Quantity: 99},
		{Name: "stock2", Quantity: 1},
	})
	if err != nil {
		t.Fatalf("set stocks: %v", err)
	}

	stocks, err := market.ListStocks(ctx)
	if err != nil {
		t.Fatalf("list stocks: %v", err)
	}
	assertStocks(t, stocks, []Stock{
		{Name: "stock1", Quantity: 99},
		{Name: "stock2", Quantity: 1},
	})

	err = market.SetStocks(ctx, []Stock{})
	if err != nil {
		t.Fatalf("clear stocks: %v", err)
	}

	stocks, err = market.ListStocks(ctx)
	if err != nil {
		t.Fatalf("list cleared stocks: %v", err)
	}
	assertStocks(t, stocks, []Stock{})
}

func TestMemoryMarketSetStocksValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		stocks []Stock
	}{
		{
			name:   "negative quantity",
			stocks: []Stock{{Name: "stock1", Quantity: -1}},
		},
		{
			name:   "empty name",
			stocks: []Stock{{Name: "", Quantity: 1}},
		},
		{
			name: "duplicate name",
			stocks: []Stock{
				{Name: "stock1", Quantity: 1},
				{Name: "stock1", Quantity: 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewMemoryMarket().SetStocks(ctx, tt.stocks)
			var validationErr ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ValidationError, got %v", err)
			}
		})
	}
}

func TestMemoryMarketBuySuccessUpdatesBankWalletAndLog(t *testing.T) {
	ctx := context.Background()
	market := NewMemoryMarket()
	setStocks(t, market, []Stock{{Name: "stock1", Quantity: 2}})

	err := market.Trade(ctx, "w1", "stock1", OperationBuy)
	if err != nil {
		t.Fatalf("buy stock: %v", err)
	}

	assertBankState(t, market, []Stock{{Name: "stock1", Quantity: 1}})
	assertWalletState(t, market, "w1", []Stock{{Name: "stock1", Quantity: 1}})
	assertLogState(t, market, []LogEntry{
		{Type: OperationBuy, WalletID: "w1", StockName: "stock1"},
	})
}

func TestMemoryMarketSellSuccessUpdatesWalletBankAndLog(t *testing.T) {
	ctx := context.Background()
	market := NewMemoryMarket()
	setStocks(t, market, []Stock{{Name: "stock1", Quantity: 1}})
	trade(t, market, "w1", "stock1", OperationBuy)

	err := market.Trade(ctx, "w1", "stock1", OperationSell)
	if err != nil {
		t.Fatalf("sell stock: %v", err)
	}

	assertBankState(t, market, []Stock{{Name: "stock1", Quantity: 1}})
	assertWalletState(t, market, "w1", []Stock{})
	assertLogState(t, market, []LogEntry{
		{Type: OperationBuy, WalletID: "w1", StockName: "stock1"},
		{Type: OperationSell, WalletID: "w1", StockName: "stock1"},
	})
}

func TestMemoryMarketTradeErrors(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		setup     func(*testing.T, *MemoryMarket)
		operation OperationType
		stockName string
		wantErr   error
	}{
		{
			name:      "unknown stock",
			operation: OperationBuy,
			stockName: "missing",
			wantErr:   ErrStockNotFound,
		},
		{
			name:      "sell unknown stock",
			operation: OperationSell,
			stockName: "missing",
			wantErr:   ErrStockNotFound,
		},
		{
			name: "insufficient bank stock",
			setup: func(t *testing.T, m *MemoryMarket) {
				setStocks(t, m, []Stock{{Name: "stock1", Quantity: 0}})
			},
			operation: OperationBuy,
			stockName: "stock1",
			wantErr:   ErrInsufficientBankStock,
		},
		{
			name: "insufficient wallet stock",
			setup: func(t *testing.T, m *MemoryMarket) {
				setStocks(t, m, []Stock{{Name: "stock1", Quantity: 1}})
			},
			operation: OperationSell,
			stockName: "stock1",
			wantErr:   ErrInsufficientWalletStock,
		},
		{
			name:      "invalid operation",
			operation: OperationType("hold"),
			stockName: "stock1",
			wantErr:   ErrInvalidOperation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			market := NewMemoryMarket()
			if tt.setup != nil {
				tt.setup(t, market)
			}

			err := market.Trade(ctx, "w1", tt.stockName, tt.operation)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
			assertLogState(t, market, []LogEntry{})
		})
	}
}

func TestMemoryMarketPostStocksDoesNotClearWalletsOrLog(t *testing.T) {
	ctx := context.Background()
	market := NewMemoryMarket()
	setStocks(t, market, []Stock{{Name: "stock1", Quantity: 1}})
	trade(t, market, "w1", "stock1", OperationBuy)

	err := market.SetStocks(ctx, []Stock{})
	if err != nil {
		t.Fatalf("clear stocks: %v", err)
	}

	assertWalletState(t, market, "w1", []Stock{{Name: "stock1", Quantity: 1}})
	assertLogState(t, market, []LogEntry{
		{Type: OperationBuy, WalletID: "w1", StockName: "stock1"},
	})
}

func TestMemoryMarketReturnsDefensiveCopies(t *testing.T) {
	ctx := context.Background()
	market := NewMemoryMarket()
	setStocks(t, market, []Stock{{Name: "stock1", Quantity: 2}})
	trade(t, market, "w1", "stock1", OperationBuy)

	stocks, err := market.ListStocks(ctx)
	if err != nil {
		t.Fatalf("list stocks: %v", err)
	}
	stocks[0].Quantity = 99
	assertBankState(t, market, []Stock{{Name: "stock1", Quantity: 1}})

	wallet, err := market.GetWallet(ctx, "w1")
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	wallet.Stocks[0].Quantity = 99
	assertWalletState(t, market, "w1", []Stock{{Name: "stock1", Quantity: 1}})

	logEntries, err := market.ListLog(ctx)
	if err != nil {
		t.Fatalf("list log: %v", err)
	}
	logEntries[0].StockName = "changed"
	assertLogState(t, market, []LogEntry{
		{Type: OperationBuy, WalletID: "w1", StockName: "stock1"},
	})
}

func setStocks(t *testing.T, market *MemoryMarket, stocks []Stock) {
	t.Helper()

	if err := market.SetStocks(context.Background(), stocks); err != nil {
		t.Fatalf("set stocks: %v", err)
	}
}

func trade(t *testing.T, market *MemoryMarket, walletID, stockName string, operation OperationType) {
	t.Helper()

	if err := market.Trade(context.Background(), walletID, stockName, operation); err != nil {
		t.Fatalf("trade %s: %v", operation, err)
	}
}

func assertBankState(t *testing.T, market *MemoryMarket, expected []Stock) {
	t.Helper()

	stocks, err := market.ListStocks(context.Background())
	if err != nil {
		t.Fatalf("list stocks: %v", err)
	}
	assertStocks(t, stocks, expected)
}

func assertWalletState(t *testing.T, market *MemoryMarket, walletID string, expected []Stock) {
	t.Helper()

	wallet, err := market.GetWallet(context.Background(), walletID)
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	assertWallet(t, wallet, Wallet{ID: walletID, Stocks: expected})
}

func assertLogState(t *testing.T, market *MemoryMarket, expected []LogEntry) {
	t.Helper()

	logEntries, err := market.ListLog(context.Background())
	if err != nil {
		t.Fatalf("list log: %v", err)
	}
	assertLog(t, logEntries, expected)
}

func assertStocks(t *testing.T, got, expected []Stock) {
	t.Helper()

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("expected stocks %#v, got %#v", expected, got)
	}
}

func assertWallet(t *testing.T, got, expected Wallet) {
	t.Helper()

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("expected wallet %#v, got %#v", expected, got)
	}
}

func assertLog(t *testing.T, got, expected []LogEntry) {
	t.Helper()

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("expected log %#v, got %#v", expected, got)
	}
}
