package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"remitly-stock-market/internal/market"
)

type Store struct {
	pool *pgxpool.Pool
}

var _ market.Market = (*Store)(nil)

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) ListStocks(ctx context.Context) ([]market.Stock, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT name, quantity
		FROM bank_stocks
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list bank stocks: %w", err)
	}
	defer rows.Close()

	stocks := []market.Stock{}
	for rows.Next() {
		var stock market.Stock
		if err := rows.Scan(&stock.Name, &stock.Quantity); err != nil {
			return nil, fmt.Errorf("scan bank stock: %w", err)
		}
		stocks = append(stocks, stock)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bank stocks: %w", err)
	}

	return stocks, nil
}

func (s *Store) SetStocks(ctx context.Context, stocks []market.Stock) error {
	if err := market.ValidateStocks(stocks); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin set stocks transaction: %w", err)
	}
	defer rollback(ctx, tx)

	if _, err := tx.Exec(ctx, `DELETE FROM bank_stocks`); err != nil {
		return fmt.Errorf("clear bank stocks: %w", err)
	}

	for _, stock := range stocks {
		if _, err := tx.Exec(ctx, `
			INSERT INTO bank_stocks (name, quantity)
			VALUES ($1, $2)
		`, stock.Name, stock.Quantity); err != nil {
			return fmt.Errorf("insert bank stock %q: %w", stock.Name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit set stocks transaction: %w", err)
	}

	return nil
}

func (s *Store) GetWallet(ctx context.Context, walletID string) (market.Wallet, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT stock_name, quantity
		FROM wallet_stocks
		WHERE wallet_id = $1
		ORDER BY stock_name
	`, walletID)
	if err != nil {
		return market.Wallet{}, fmt.Errorf("get wallet stocks: %w", err)
	}
	defer rows.Close()

	wallet := market.Wallet{
		ID:     walletID,
		Stocks: []market.Stock{},
	}
	for rows.Next() {
		var stock market.Stock
		if err := rows.Scan(&stock.Name, &stock.Quantity); err != nil {
			return market.Wallet{}, fmt.Errorf("scan wallet stock: %w", err)
		}
		wallet.Stocks = append(wallet.Stocks, stock)
	}
	if err := rows.Err(); err != nil {
		return market.Wallet{}, fmt.Errorf("iterate wallet stocks: %w", err)
	}

	return wallet, nil
}

func (s *Store) GetWalletStock(ctx context.Context, walletID, stockName string) (int, error) {
	var quantity int
	err := s.pool.QueryRow(ctx, `
		SELECT quantity
		FROM wallet_stocks
		WHERE wallet_id = $1 AND stock_name = $2
	`, walletID, stockName).Scan(&quantity)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get wallet stock: %w", err)
	}

	return quantity, nil
}

func (s *Store) Trade(ctx context.Context, walletID, stockName string, operation market.OperationType) error {
	if operation != market.OperationBuy && operation != market.OperationSell {
		return market.ErrInvalidOperation
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin trade transaction: %w", err)
	}
	defer rollback(ctx, tx)

	var bankQuantity int
	err = tx.QueryRow(ctx, `
		SELECT quantity
		FROM bank_stocks
		WHERE name = $1
		FOR UPDATE
	`, stockName).Scan(&bankQuantity)
	if errors.Is(err, pgx.ErrNoRows) {
		return market.ErrStockNotFound
	}
	if err != nil {
		return fmt.Errorf("lock bank stock: %w", err)
	}

	switch operation {
	case market.OperationBuy:
		if bankQuantity <= 0 {
			return market.ErrInsufficientBankStock
		}
		if err := buy(ctx, tx, walletID, stockName); err != nil {
			return err
		}
	case market.OperationSell:
		if err := sell(ctx, tx, walletID, stockName); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (type, wallet_id, stock_name)
		VALUES ($1, $2, $3)
	`, string(operation), walletID, stockName); err != nil {
		return fmt.Errorf("append audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit trade transaction: %w", err)
	}

	return nil
}

func (s *Store) ListLog(ctx context.Context) ([]market.LogEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT type, wallet_id, stock_name
		FROM audit_log
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list audit log: %w", err)
	}
	defer rows.Close()

	logEntries := []market.LogEntry{}
	for rows.Next() {
		var entry market.LogEntry
		if err := rows.Scan(&entry.Type, &entry.WalletID, &entry.StockName); err != nil {
			return nil, fmt.Errorf("scan audit log entry: %w", err)
		}
		logEntries = append(logEntries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit log: %w", err)
	}

	return logEntries, nil
}

func buy(ctx context.Context, tx pgx.Tx, walletID, stockName string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE bank_stocks
		SET quantity = quantity - 1
		WHERE name = $1
	`, stockName); err != nil {
		return fmt.Errorf("decrement bank stock: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO wallets (id)
		VALUES ($1)
		ON CONFLICT (id) DO NOTHING
	`, walletID); err != nil {
		return fmt.Errorf("ensure wallet: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO wallet_stocks (wallet_id, stock_name, quantity)
		VALUES ($1, $2, 1)
		ON CONFLICT (wallet_id, stock_name)
		DO UPDATE SET quantity = wallet_stocks.quantity + 1
	`, walletID, stockName); err != nil {
		return fmt.Errorf("increment wallet stock: %w", err)
	}

	return nil
}

func sell(ctx context.Context, tx pgx.Tx, walletID, stockName string) error {
	var walletQuantity int
	err := tx.QueryRow(ctx, `
		SELECT quantity
		FROM wallet_stocks
		WHERE wallet_id = $1 AND stock_name = $2
		FOR UPDATE
	`, walletID, stockName).Scan(&walletQuantity)
	if errors.Is(err, pgx.ErrNoRows) {
		return market.ErrInsufficientWalletStock
	}
	if err != nil {
		return fmt.Errorf("lock wallet stock: %w", err)
	}

	if walletQuantity <= 1 {
		if _, err := tx.Exec(ctx, `
			DELETE FROM wallet_stocks
			WHERE wallet_id = $1 AND stock_name = $2
		`, walletID, stockName); err != nil {
			return fmt.Errorf("delete wallet stock: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE wallet_stocks
			SET quantity = quantity - 1
			WHERE wallet_id = $1 AND stock_name = $2
		`, walletID, stockName); err != nil {
			return fmt.Errorf("decrement wallet stock: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE bank_stocks
		SET quantity = quantity + 1
		WHERE name = $1
	`, stockName); err != nil {
		return fmt.Errorf("increment bank stock: %w", err)
	}

	return nil
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}
