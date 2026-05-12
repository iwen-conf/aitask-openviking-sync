#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-}"
ENV_FILE="${ENV_FILE:-.env}"
POSTGRES_SERVICE="${POSTGRES_SERVICE:-postgres}"
MIGRATIONS_DIR="${MIGRATIONS_DIR:-migrations/postgres}"
POSTGRES_WAIT_SECONDS="${POSTGRES_WAIT_SECONDS:-90}"
MIGRATE_GO_VERSION="${MIGRATE_GO_VERSION:-v4.19.0}"

usage() {
  cat <<'USAGE'
Usage:
  bash scripts/migrate.sh [command] [arg]

Commands:
  up [N]         Apply all up migrations (or next N migrations).
  down [N]       Revert one migration by default, or N migrations.
  goto VERSION   Migrate to specific version (up or down).
  force VERSION  Force set migration version.
  version        Print current migration version.

Defaults:
  command defaults to `up`.

Environment:
  ENV_FILE             env file path (default: .env)
  COMPOSE_FILE         compose file path (default: docker-compose.yml)
  MIGRATIONS_DIR       migration dir (default: migrations/postgres)
  MIGRATE_DATABASE_URL explicit DB URL for migrate tool
USAGE
}

if [[ -f "$ENV_FILE" ]]; then
  set -a
  source "$ENV_FILE"
  set +a
fi

POSTGRES_USER="${POSTGRES_USER:-aitask}"
POSTGRES_DB="${POSTGRES_DB:-aitask}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-aitask_dev_password}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"

MIGRATE_DATABASE_URL="${MIGRATE_DATABASE_URL:-postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@127.0.0.1:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable}"

compose_cmd=(docker compose -f "$COMPOSE_FILE")
if [[ -n "$COMPOSE_PROJECT_NAME" ]]; then
  compose_cmd=(docker compose -p "$COMPOSE_PROJECT_NAME" -f "$COMPOSE_FILE")
fi
if [[ -f "$ENV_FILE" ]]; then
  compose_cmd+=(--env-file "$ENV_FILE")
fi

if [[ ! -d "$MIGRATIONS_DIR" ]]; then
  echo "[migrate] migration directory not found: $MIGRATIONS_DIR"
  exit 1
fi

if ! find "$MIGRATIONS_DIR" -maxdepth 1 -type f -name '*.sql' | grep -q .; then
  echo "[migrate] no SQL files found in $MIGRATIONS_DIR, skip."
  exit 0
fi

echo "[migrate] waiting for postgres service '$POSTGRES_SERVICE'..."
deadline=$((SECONDS + POSTGRES_WAIT_SECONDS))
until "${compose_cmd[@]}" exec -T \
  -e "PGPASSWORD=$POSTGRES_PASSWORD" \
  "$POSTGRES_SERVICE" \
  pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; do
  if (( SECONDS >= deadline )); then
    echo "[migrate] postgres not ready within ${POSTGRES_WAIT_SECONDS}s"
    exit 1
  fi
  sleep 1
done

migrate_cmd=()
migrate_pkg=""
if command -v migrate >/dev/null 2>&1; then
  migrate_cmd=(migrate)
else
  migrate_cmd=(go run -tags postgres)
  migrate_pkg="github.com/golang-migrate/migrate/v4/cmd/migrate@${MIGRATE_GO_VERSION}"
fi

run_migrate() {
  if [[ "${migrate_cmd[0]}" == "go" ]]; then
    GOFLAGS="-mod=mod" "${migrate_cmd[@]}" "$migrate_pkg" -path "$MIGRATIONS_DIR" -database "$MIGRATE_DATABASE_URL" "$@"
    return
  fi
  "${migrate_cmd[@]}" -path "$MIGRATIONS_DIR" -database "$MIGRATE_DATABASE_URL" "$@"
}

command_name="${1:-up}"
arg="${2:-}"

case "$command_name" in
  up)
    if [[ -n "$arg" ]]; then
      echo "[migrate] running: up $arg"
      run_migrate up "$arg"
    else
      echo "[migrate] running: up"
      run_migrate up
    fi
    ;;
  down)
    if [[ -n "$arg" ]]; then
      echo "[migrate] running: down $arg"
      run_migrate down "$arg"
    else
      echo "[migrate] running: down 1"
      run_migrate down 1
    fi
    ;;
  goto)
    if [[ -z "$arg" ]]; then
      echo "[migrate] goto requires VERSION"
      usage
      exit 1
    fi
    echo "[migrate] running: goto $arg"
    run_migrate goto "$arg"
    ;;
  force)
    if [[ -z "$arg" ]]; then
      echo "[migrate] force requires VERSION"
      usage
      exit 1
    fi
    echo "[migrate] running: force $arg"
    run_migrate force "$arg"
    ;;
  version)
    echo "[migrate] running: version"
    run_migrate version
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    echo "[migrate] unknown command: $command_name"
    usage
    exit 1
    ;;
esac

echo "[migrate] done."
