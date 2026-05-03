CREATE TABLE IF NOT EXISTS bank_stocks (
    name text PRIMARY KEY,
    quantity integer NOT NULL CHECK (quantity >= 0)
);

CREATE TABLE IF NOT EXISTS wallets (
    id text PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS wallet_stocks (
    wallet_id text NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,
    stock_name text NOT NULL,
    quantity integer NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (wallet_id, stock_name)
);

CREATE TABLE IF NOT EXISTS audit_log (
    id bigserial PRIMARY KEY,
    type text NOT NULL CHECK (type IN ('buy', 'sell')),
    wallet_id text NOT NULL,
    stock_name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
