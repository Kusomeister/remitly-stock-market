# Remitly Stock Market

A simplified stock market service implemented in Go. The market has three core concepts:

- Bank: the single liquidity provider that owns the available stock inventory.
- Wallets: user-owned portfolios that can buy and sell single stock units.
- Audit log: successful wallet operations in order of occurrence. Bank setup operations are not logged.

The service follows the task assumptions: stock price is fixed at 1, wallet cash balance is not tracked, and every buy or sell operation is executed immediately against the Bank.

## Quick Start

Requirements:

- Docker with Docker Compose
- Go 1.26, only needed for running tests or the service outside Docker

Start the full stack from a clean environment:

```powershell
.\run.ps1 -Port 8080
```

```sh
./run.sh 8080
```

The startup scripts first run:

```sh
docker compose --profile observability down --volumes --remove-orphans
```

This intentionally removes all project containers and volumes before starting the stack, including Postgres, Prometheus, Loki, Alloy, and Grafana data. Every scripted startup begins with an empty Bank, no wallets, and an empty audit log.

Then the scripts start the high-availability stack with two app instances:

```sh
docker compose --profile observability up --build --scale app=2
```

## Service URLs

- API: `http://localhost:<port>` where `<port>` is the startup command parameter.
- Grafana: `http://localhost:3000` by default, login `admin` / `admin`.
- App metrics: exposed by each app instance on `:9091/metrics` inside Docker and scraped by Prometheus.

To change the Grafana port:

```powershell
.\run.ps1 -Port 8080 -GrafanaPort 3001
```

```sh
GRAFANA_PORT=3001 ./run.sh 8080
```

## API

All responses are JSON. Successful mutating endpoints return HTTP 200 with an empty JSON object.

### Set Bank Stocks

```http
POST /stocks
```

Body:

```json
{
  "stocks": [
    {"name": "AAPL", "quantity": 10},
    {"name": "MSFT", "quantity": 5}
  ]
}
```

Replaces the Bank inventory. Stock names must be unique and non-empty, and quantities must not be negative.

### Get Bank Stocks

```http
GET /stocks
```

Response:

```json
{
  "stocks": [
    {"name": "AAPL", "quantity": 10}
  ]
}
```

### Buy Or Sell One Stock

```http
POST /wallets/{wallet_id}/stocks/{stock_name}
```

Body:

```json
{"type": "buy"}
```

or:

```json
{"type": "sell"}
```

Behavior:

- Unknown stock returns HTTP 404.
- Buying a stock with zero Bank quantity returns HTTP 400.
- Selling a stock missing from the wallet returns HTTP 400.
- A successful operation returns HTTP 200, updates the Bank quantity, updates the wallet, and appends one audit log entry.
- A missing wallet is created automatically by a successful buy.

### Get Wallet

```http
GET /wallets/{wallet_id}
```

Response:

```json
{
  "id": "wallet-1",
  "stocks": [
    {"name": "AAPL", "quantity": 1}
  ]
}
```

Missing wallets return an empty wallet response.

### Get Wallet Stock Quantity

```http
GET /wallets/{wallet_id}/stocks/{stock_name}
```

Response:

```json
1
```

Missing wallet stocks return `0`.

### Get Audit Log

```http
GET /log
```

Response:

```json
{
  "log": [
    {"type": "buy", "wallet_id": "wallet-1", "stock_name": "AAPL"},
    {"type": "sell", "wallet_id": "wallet-1", "stock_name": "AAPL"}
  ]
}
```

Only successful wallet buy and sell operations are logged.

### Kill Current Instance

```http
POST /chaos
```

Returns HTTP 200, then exits the app instance that served the request. Traefik continues routing to the remaining app instance while Docker Compose restarts the killed one.

### Health Check

```http
GET /health
```

Response:

```json
{"status":"ok"}
```

## Example Flow

Set initial Bank stock:

```sh
curl -i -X POST http://localhost:8080/stocks \
  -H "Content-Type: application/json" \
  -d '{"stocks":[{"name":"AAPL","quantity":2},{"name":"MSFT","quantity":1}]}'
```

Buy one stock:

```sh
curl -i -X POST http://localhost:8080/wallets/wallet-1/stocks/AAPL \
  -H "Content-Type: application/json" \
  -d '{"type":"buy"}'
```

Check Bank state:

```sh
curl http://localhost:8080/stocks
```

Check wallet state:

```sh
curl http://localhost:8080/wallets/wallet-1
```

Check one wallet stock quantity:

```sh
curl http://localhost:8080/wallets/wallet-1/stocks/AAPL
```

Sell one stock:

```sh
curl -i -X POST http://localhost:8080/wallets/wallet-1/stocks/AAPL \
  -H "Content-Type: application/json" \
  -d '{"type":"sell"}'
```

Read the audit log:

```sh
curl http://localhost:8080/log
```

Trigger chaos and verify the product stays available:

```sh
curl -i -X POST http://localhost:8080/chaos
curl -i http://localhost:8080/health
```

## Architecture

- The app is a Go 1.26 HTTP service using the standard `net/http` package.
- HTTP handlers depend on a `market.Market` interface, keeping API code separate from storage.
- The production Docker Compose setup uses Postgres as the shared store for all app instances.
- An in-memory market implementation is available for unit tests and local fallback when `DATABASE_URL` is not set.
- Traefik exposes the API on `localhost:<port>` and load-balances two app instances.
- `POST /chaos` terminates only the instance that handled the request; the second instance keeps the API available and Compose restarts the killed process.
- Observability is included through Prometheus, Grafana, Loki, Alloy, and Postgres exporter.
- The app emits JSON request logs and Prometheus HTTP metrics.

## Tests

Run the full test suite:

```sh
go test -count=1 ./...
```

The Postgres integration tests use Testcontainers and require Docker access from the test process. GitHub Actions runs:

```sh
go test ./...
```

Useful local checks:

```sh
docker compose --profile observability config --quiet
go test -count=1 ./...
```

## Operational Notes

- The provided startup scripts are the recommended way to run the project because they create a clean environment and start the complete HA and observability stack.
- App state is intentionally reset by the scripts. To inspect persistent behavior manually, run Docker Compose directly without the scripted `down --volumes` step.
