#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
ENV_FILE="${ENV_FILE:-.env}"
ENV_EXAMPLE_FILE="${ENV_EXAMPLE_FILE:-.env.example}"
DEV_UP_SERVICES="${DEV_UP_SERVICES:-postgres dragonfly backend openviking}"
READYZ_WAIT_SECONDS="${READYZ_WAIT_SECONDS:-120}"
DEV_UP_BUILD="${DEV_UP_BUILD:-1}"

if [[ ! -f "$ENV_FILE" ]]; then
  if [[ ! -f "$ENV_EXAMPLE_FILE" ]]; then
    echo "[dev-up] missing $ENV_FILE and $ENV_EXAMPLE_FILE"
    exit 1
  fi
  cp "$ENV_EXAMPLE_FILE" "$ENV_FILE"
  echo "[dev-up] created $ENV_FILE from $ENV_EXAMPLE_FILE"
fi

set -a
source "$ENV_FILE"
set +a

APP_PORT="${APP_PORT:-8080}"
READYZ_URL="http://127.0.0.1:${APP_PORT}/readyz"
compose_cmd=(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE")

echo "[dev-up] starting services: $DEV_UP_SERVICES"
if [[ "$DEV_UP_BUILD" == "1" ]]; then
  "${compose_cmd[@]}" up -d --build $DEV_UP_SERVICES
else
  "${compose_cmd[@]}" up -d $DEV_UP_SERVICES
fi

echo "[dev-up] running migrations"
ENV_FILE="$ENV_FILE" COMPOSE_FILE="$COMPOSE_FILE" bash "$ROOT_DIR/scripts/migrate.sh"

echo "[dev-up] waiting for backend readiness: $READYZ_URL"
deadline=$((SECONDS + READYZ_WAIT_SECONDS))
while true; do
  body="$(curl -fsS "$READYZ_URL" 2>/dev/null || true)"
  if [[ -n "$body" ]] && [[ "$body" =~ \"status\":\"(ready|degraded)\" ]]; then
    echo "[dev-up] backend ready: $body"
    break
  fi
  if (( SECONDS >= deadline )); then
    echo "[dev-up] backend did not become ready within ${READYZ_WAIT_SECONDS}s"
    "${compose_cmd[@]}" ps
    exit 1
  fi
  sleep 1
done

"${compose_cmd[@]}" ps
echo "[dev-up] done."
