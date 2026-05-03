package market

import (
	"context"
	"errors"
	"strings"
	"sync"
)

type Stock struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

type Wallet struct {
	ID     string  `json:"id"`
	Stocks []Stock `json:"stocks"`
}

type OperationType string

const (
	OperationBuy  OperationType = "buy"
	OperationSell OperationType = "sell"
)

type LogEntry struct {
	Type      OperationType `json:"type"`
	WalletID  string        `json:"wallet_id"`
	StockName string        `json:"stock_name"`
}

var (
	ErrStockNotFound           = errors.New("stock not found")
	ErrInsufficientBankStock   = errors.New("insufficient bank stock")
	ErrInsufficientWalletStock = errors.New("insufficient wallet stock")
	ErrInvalidOperation        = errors.New("invalid operation")
)

type Market interface {
	ListStocks(ctx context.Context) ([]Stock, error)
	SetStocks(ctx context.Context, stocks []Stock) error
	GetWallet(ctx context.Context, walletID string) (Wallet, error)
	GetWalletStock(ctx context.Context, walletID, stockName string) (int, error)
	Trade(ctx context.Context, walletID, stockName string, operation OperationType) error
	ListLog(ctx context.Context) ([]LogEntry, error)
}

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

type MemoryMarket struct {
	mu           sync.RWMutex
	stocks       []Stock
	walletStocks map[string][]Stock
	log          []LogEntry
}

var _ Market = (*MemoryMarket)(nil)

func NewMemoryMarket() *MemoryMarket {
	return &MemoryMarket{
		walletStocks: make(map[string][]Stock),
	}
}

func (m *MemoryMarket) ListStocks(ctx context.Context) ([]Stock, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return cloneStocks(m.stocks), nil
}

func (m *MemoryMarket) SetStocks(ctx context.Context, stocks []Stock) error {
	if err := validateStocks(stocks); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.stocks = cloneStocks(stocks)
	return nil
}

func (m *MemoryMarket) GetWallet(ctx context.Context, walletID string) (Wallet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return Wallet{
		ID:     walletID,
		Stocks: cloneStocks(m.walletStocks[walletID]),
	}, nil
}

func (m *MemoryMarket) GetWalletStock(ctx context.Context, walletID, stockName string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return stockQuantity(m.walletStocks[walletID], stockName), nil
}

func (m *MemoryMarket) Trade(ctx context.Context, walletID, stockName string, operation OperationType) error {
	if operation != OperationBuy && operation != OperationSell {
		return ErrInvalidOperation
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	bankStockIndex := stockIndex(m.stocks, stockName)
	if bankStockIndex == -1 {
		return ErrStockNotFound
	}

	switch operation {
	case OperationBuy:
		if m.stocks[bankStockIndex].Quantity <= 0 {
			return ErrInsufficientBankStock
		}

		m.stocks[bankStockIndex].Quantity--
		m.addWalletStock(walletID, stockName, 1)
		m.appendLog(OperationBuy, walletID, stockName)
	case OperationSell:
		if stockQuantity(m.walletStocks[walletID], stockName) <= 0 {
			return ErrInsufficientWalletStock
		}

		m.addWalletStock(walletID, stockName, -1)
		m.stocks[bankStockIndex].Quantity++
		m.appendLog(OperationSell, walletID, stockName)
	}

	return nil
}

func (m *MemoryMarket) ListLog(ctx context.Context) ([]LogEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.log) == 0 {
		return []LogEntry{}, nil
	}

	cloned := make([]LogEntry, len(m.log))
	copy(cloned, m.log)
	return cloned, nil
}

func (m *MemoryMarket) addWalletStock(walletID, stockName string, delta int) {
	if m.walletStocks == nil {
		m.walletStocks = make(map[string][]Stock)
	}

	stocks := m.walletStocks[walletID]
	stockIndex := stockIndex(stocks, stockName)
	if stockIndex == -1 {
		if delta <= 0 {
			return
		}
		m.walletStocks[walletID] = append(stocks, Stock{Name: stockName, Quantity: delta})
		return
	}

	stocks[stockIndex].Quantity += delta
	if stocks[stockIndex].Quantity <= 0 {
		stocks = append(stocks[:stockIndex], stocks[stockIndex+1:]...)
	}

	m.walletStocks[walletID] = stocks
}

func (m *MemoryMarket) appendLog(operation OperationType, walletID, stockName string) {
	m.log = append(m.log, LogEntry{
		Type:      operation,
		WalletID:  walletID,
		StockName: stockName,
	})
}

func validateStocks(stocks []Stock) error {
	seen := make(map[string]struct{}, len(stocks))
	for _, stock := range stocks {
		if strings.TrimSpace(stock.Name) == "" {
			return ValidationError{Message: "stock name must not be empty"}
		}
		if stock.Quantity < 0 {
			return ValidationError{Message: "stock quantity must not be negative"}
		}
		if _, ok := seen[stock.Name]; ok {
			return ValidationError{Message: "stock names must be unique"}
		}
		seen[stock.Name] = struct{}{}
	}

	return nil
}

func stockIndex(stocks []Stock, name string) int {
	for i, stock := range stocks {
		if stock.Name == name {
			return i
		}
	}

	return -1
}

func stockQuantity(stocks []Stock, name string) int {
	index := stockIndex(stocks, name)
	if index == -1 {
		return 0
	}

	return stocks[index].Quantity
}

func cloneStocks(stocks []Stock) []Stock {
	if len(stocks) == 0 {
		return []Stock{}
	}

	cloned := make([]Stock, len(stocks))
	copy(cloned, stocks)
	return cloned
}
