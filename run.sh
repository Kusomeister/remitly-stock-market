#!/usr/bin/env sh
set -eu

PORT="${1:-8080}"
export PORT

GRAFANA_PORT="${GRAFANA_PORT:-3000}"
export GRAFANA_PORT

SCRIPT_DIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
cd "$SCRIPT_DIR"

docker compose --profile observability down --volumes --remove-orphans
docker compose --profile observability up --build --scale app=2
