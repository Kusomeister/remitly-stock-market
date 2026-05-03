package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"remitly-stock-market/internal/market"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	ctx := context.Background()
	cleanupDockerConfig := useEmptyDockerConfig()
	defer cleanupDockerConfig()

	container, err := postgrescontainer.Run(ctx,
		"postgres:16-alpine",
		postgrescontainer.WithDatabase("remitly_test"),
		postgrescontainer.WithUsername("postgres"),
		postgrescontainer.WithPassword("postgres"),
		postgrescontainer.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres container: %v\n", err)
		return 1
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			fmt.Fprintf(os.Stderr, "terminate postgres container: %v\n", err)
		}
	}()

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "get postgres connection string: %v\n", err)
		return 1
	}

	testPool, err = pgxpool.New(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create postgres pool: %v\n", err)
		return 1
	}
	defer testPool.Close()

	if err := Migrate(ctx, testPool); err != nil {
		fmt.Fprintf(os.Stderr, "run migrations: %v\n", err)
		return 1
	}

	return m.Run()
}

func TestMigrateIsIdempotent(t *testing.T) {
	if err := Migrate(context.Background(), testPool); err != nil {
		t.Fatalf("run migrations again: %v", err)
	}
}

func TestStoreSetStocks(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.SetStocks(ctx, []market.Stock{
		{Name: "stock2", Quantity: 1},
		{Name: "stock1", Quantity: 99},
	})
	if err != nil {
		t.Fatalf("set stocks: %v", err)
	}

	stocks, err := store.ListStocks(ctx)
	if err != nil {
		t.Fatalf("list stocks: %v", err)
	}
	assertStockSet(t, stocks, []market.Stock{
		{Name: "stock1", Quantity: 99},
		{Name: "stock2", Quantity: 1},
	})

	err = store.SetStocks(ctx, []market.Stock{})
	if err != nil {
		t.Fatalf("clear stocks: %v", err)
	}

	stocks, err = store.ListStocks(ctx)
	if err != nil {
		t.Fatalf("list cleared stocks: %v", err)
	}
	assertStockSet(t, stocks, []market.Stock{})
}

func TestStoreSetStocksValidation(t *testing.T) {
	store := newTestStore(t)

	err := store.SetStocks(context.Background(), []market.Stock{{Name: "", Quantity: 1}})
	var validationErr market.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestStoreBuySuccessUpdatesBankWalletAndLog(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	setStocks(t, store, []market.Stock{{Name: "stock1", Quantity: 2}})

	err := store.Trade(ctx, "w1", "stock1", market.OperationBuy)
	if err != nil {
		t.Fatalf("buy stock: %v", err)
	}

	assertBankState(t, store, []market.Stock{{Name: "stock1", Quantity: 1}})
	assertWalletState(t, store, "w1", []market.Stock{{Name: "stock1", Quantity: 1}})
	assertLogState(t, store, []market.LogEntry{
		{Type: market.OperationBuy, WalletID: "w1", StockName: "stock1"},
	})
}

func TestStoreSellSuccessUpdatesWalletBankAndLog(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	setStocks(t, store, []market.Stock{{Name: "stock1", Quantity: 1}})
	trade(t, store, "w1", "stock1", market.OperationBuy)

	err := store.Trade(ctx, "w1", "stock1", market.OperationSell)
	if err != nil {
		t.Fatalf("sell stock: %v", err)
	}

	assertBankState(t, store, []market.Stock{{Name: "stock1", Quantity: 1}})
	assertWalletState(t, store, "w1", []market.Stock{})
	assertLogState(t, store, []market.LogEntry{
		{Type: market.OperationBuy, WalletID: "w1", StockName: "stock1"},
		{Type: market.OperationSell, WalletID: "w1", StockName: "stock1"},
	})
}

func TestStoreTradeErrors(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*testing.T, *Store)
		operation market.OperationType
		stockName string
		wantErr   error
	}{
		{
			name:      "buy unknown stock",
			operation: market.OperationBuy,
			stockName: "missing",
			wantErr:   market.ErrStockNotFound,
		},
		{
			name:      "sell unknown stock",
			operation: market.OperationSell,
			stockName: "missing",
			wantErr:   market.ErrStockNotFound,
		},
		{
			name: "insufficient bank stock",
			setup: func(t *testing.T, store *Store) {
				setStocks(t, store, []market.Stock{{Name: "stock1", Quantity: 0}})
			},
			operation: market.OperationBuy,
			stockName: "stock1",
			wantErr:   market.ErrInsufficientBankStock,
		},
		{
			name: "insufficient wallet stock",
			setup: func(t *testing.T, store *Store) {
				setStocks(t, store, []market.Stock{{Name: "stock1", Quantity: 1}})
			},
			operation: market.OperationSell,
			stockName: "stock1",
			wantErr:   market.ErrInsufficientWalletStock,
		},
		{
			name:      "invalid operation",
			operation: market.OperationType("hold"),
			stockName: "stock1",
			wantErr:   market.ErrInvalidOperation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			if tt.setup != nil {
				tt.setup(t, store)
			}

			err := store.Trade(context.Background(), "w1", tt.stockName, tt.operation)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
			assertLogState(t, store, []market.LogEntry{})
		})
	}
}

func TestStoreAuditLogPreservesOrder(t *testing.T) {
	store := newTestStore(t)
	setStocks(t, store, []market.Stock{{Name: "stock1", Quantity: 2}})
	trade(t, store, "w1", "stock1", market.OperationBuy)
	trade(t, store, "w1", "stock1", market.OperationSell)
	trade(t, store, "w2", "stock1", market.OperationBuy)

	assertLogState(t, store, []market.LogEntry{
		{Type: market.OperationBuy, WalletID: "w1", StockName: "stock1"},
		{Type: market.OperationSell, WalletID: "w1", StockName: "stock1"},
		{Type: market.OperationBuy, WalletID: "w2", StockName: "stock1"},
	})
}

func TestStoreSetStocksDoesNotClearWalletsOrLog(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	setStocks(t, store, []market.Stock{{Name: "stock1", Quantity: 1}})
	trade(t, store, "w1", "stock1", market.OperationBuy)

	err := store.SetStocks(ctx, []market.Stock{})
	if err != nil {
		t.Fatalf("clear stocks: %v", err)
	}

	assertWalletState(t, store, "w1", []market.Stock{{Name: "stock1", Quantity: 1}})
	assertLogState(t, store, []market.LogEntry{
		{Type: market.OperationBuy, WalletID: "w1", StockName: "stock1"},
	})
}

func TestStoreGetMissingWalletAndStock(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	wallet, err := store.GetWallet(ctx, "w1")
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	assertWallet(t, wallet, market.Wallet{ID: "w1", Stocks: []market.Stock{}})

	quantity, err := store.GetWalletStock(ctx, "w1", "stock1")
	if err != nil {
		t.Fatalf("get wallet stock: %v", err)
	}
	if quantity != 0 {
		t.Fatalf("expected missing stock quantity 0, got %d", quantity)
	}
}

func TestStoreConcurrentBuysDoNotOversell(t *testing.T) {
	store := newTestStore(t)
	setStocks(t, store, []market.Stock{{Name: "stock1", Quantity: 1}})

	const attempts = 10
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- store.Trade(context.Background(), fmt.Sprintf("w%d", i), "stock1", market.OperationBuy)
		}(i)
	}
	wg.Wait()
	close(errs)

	var successes int
	var insufficientBankStock int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, market.ErrInsufficientBankStock):
			insufficientBankStock++
		default:
			t.Fatalf("unexpected trade error: %v", err)
		}
	}

	if successes != 1 {
		t.Fatalf("expected 1 successful buy, got %d", successes)
	}
	if insufficientBankStock != attempts-1 {
		t.Fatalf("expected %d insufficient bank stock errors, got %d", attempts-1, insufficientBankStock)
	}

	assertBankState(t, store, []market.Stock{{Name: "stock1", Quantity: 0}})
	logEntries, err := store.ListLog(context.Background())
	if err != nil {
		t.Fatalf("list log: %v", err)
	}
	if len(logEntries) != 1 {
		t.Fatalf("expected 1 audit log entry, got %#v", logEntries)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()

	_, err := testPool.Exec(context.Background(), `
		TRUNCATE audit_log, wallet_stocks, wallets, bank_stocks RESTART IDENTITY
	`)
	if err != nil {
		t.Fatalf("truncate tables: %v", err)
	}

	return NewStore(testPool)
}

func setStocks(t *testing.T, store *Store, stocks []market.Stock) {
	t.Helper()

	if err := store.SetStocks(context.Background(), stocks); err != nil {
		t.Fatalf("set stocks: %v", err)
	}
}

func trade(t *testing.T, store *Store, walletID, stockName string, operation market.OperationType) {
	t.Helper()

	if err := store.Trade(context.Background(), walletID, stockName, operation); err != nil {
		t.Fatalf("trade %s: %v", operation, err)
	}
}

func assertBankState(t *testing.T, store *Store, expected []market.Stock) {
	t.Helper()

	stocks, err := store.ListStocks(context.Background())
	if err != nil {
		t.Fatalf("list stocks: %v", err)
	}
	assertStockSet(t, stocks, expected)
}

func assertWalletState(t *testing.T, store *Store, walletID string, expected []market.Stock) {
	t.Helper()

	wallet, err := store.GetWallet(context.Background(), walletID)
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	assertWallet(t, wallet, market.Wallet{ID: walletID, Stocks: expected})
}

func assertLogState(t *testing.T, store *Store, expected []market.LogEntry) {
	t.Helper()

	logEntries, err := store.ListLog(context.Background())
	if err != nil {
		t.Fatalf("list log: %v", err)
	}
	if !reflect.DeepEqual(logEntries, expected) {
		t.Fatalf("expected log %#v, got %#v", expected, logEntries)
	}
}

func assertWallet(t *testing.T, got, expected market.Wallet) {
	t.Helper()

	if got.ID != expected.ID {
		t.Fatalf("expected wallet ID %q, got %q", expected.ID, got.ID)
	}
	assertStockSet(t, got.Stocks, expected.Stocks)
}

func assertStockSet(t *testing.T, got, expected []market.Stock) {
	t.Helper()

	gotByName := stockSet(t, got)
	expectedByName := stockSet(t, expected)
	if !reflect.DeepEqual(gotByName, expectedByName) {
		t.Fatalf("expected stock set %#v, got %#v", expectedByName, gotByName)
	}
}

func stockSet(t *testing.T, stocks []market.Stock) map[string]int {
	t.Helper()

	byName := make(map[string]int, len(stocks))
	for _, stock := range stocks {
		if _, exists := byName[stock.Name]; exists {
			t.Fatalf("duplicate stock %q in %#v", stock.Name, stocks)
		}
		byName[stock.Name] = stock.Quantity
	}

	return byName
}

func useEmptyDockerConfig() func() {
	if os.Getenv("DOCKER_CONFIG") != "" {
		return func() {}
	}

	dir, err := os.MkdirTemp("", "remitly-docker-config-*")
	if err != nil {
		return func() {}
	}
	os.Setenv("DOCKER_CONFIG", dir)

	return func() {
		os.Unsetenv("DOCKER_CONFIG")
		_ = os.RemoveAll(dir)
	}
}
