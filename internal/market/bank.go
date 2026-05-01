package market

import (
	"context"
	"strings"
	"sync"
)

type Stock struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

type BankStocks interface {
	ListStocks(ctx context.Context) ([]Stock, error)
	SetStocks(ctx context.Context, stocks []Stock) error
}

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

type MemoryBank struct {
	mu     sync.RWMutex
	stocks []Stock
}

func NewMemoryBank() *MemoryBank {
	return &MemoryBank{}
}

func (b *MemoryBank) ListStocks(ctx context.Context) ([]Stock, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return cloneStocks(b.stocks), nil
}

func (b *MemoryBank) SetStocks(ctx context.Context, stocks []Stock) error {
	if err := validateStocks(stocks); err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.stocks = cloneStocks(stocks)
	return nil
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

func cloneStocks(stocks []Stock) []Stock {
	if len(stocks) == 0 {
		return []Stock{}
	}

	cloned := make([]Stock, len(stocks))
	copy(cloned, stocks)
	return cloned
}
