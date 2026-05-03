package market

import (
	"context"
	"errors"
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
