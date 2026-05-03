#!/usr/bin/env sh
set -eu

PORT="${1:-8080}"
export PORT

SCRIPT_DIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
cd "$SCRIPT_DIR"

docker compose up --build --scale app=2
