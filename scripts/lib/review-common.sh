#!/usr/bin/env bash
set -euo pipefail

if [[ -n "${AITASK_REVIEW_COMMON_LOADED:-}" ]]; then
  return 0
fi
AITASK_REVIEW_COMMON_LOADED=1

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ARTIFACTS_ROOT="${ARTIFACTS_ROOT:-$ROOT_DIR/.artifacts/review}"
RUNTIME_ROOT="${RUNTIME_ROOT:-$ARTIFACTS_ROOT/runtime}"
PERF_ROOT="${PERF_ROOT:-$ARTIFACTS_ROOT/perf}"
DIFF_ROOT="${DIFF_ROOT:-$ARTIFACTS_ROOT/diff}"
TMP_ROOT="${TMP_ROOT:-$ARTIFACTS_ROOT/tmp}"
REVIEW_ENV_FILE="${REVIEW_ENV_FILE:-$TMP_ROOT/review.env}"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT_DIR/docker-compose.yml}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-aitaskreview}"
APP_PORT="${APP_PORT:-18180}"
WEB_PORT="${WEB_PORT:-18100}"
POSTGRES_PORT="${POSTGRES_PORT:-15532}"
DRAGONFLY_PORT="${DRAGONFLY_PORT:-17479}"
OPENVIKING_PORT="${OPENVIKING_PORT:-19290}"
POSTGRES_DB="${POSTGRES_DB:-aitask}"
POSTGRES_USER="${POSTGRES_USER:-aitask}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-aitask_dev_password}"
DRAGONFLY_PASSWORD="${DRAGONFLY_PASSWORD:-dragonfly_dev_password}"
REVIEW_WAIT_SECONDS="${REVIEW_WAIT_SECONDS:-180}"
REVIEW_BUILD="${REVIEW_BUILD:-1}"
REVIEW_CLI_FORMAT="${REVIEW_CLI_FORMAT:-json}"
REVIEW_SERVER_URL="${REVIEW_SERVER_URL:-http://127.0.0.1:${APP_PORT}}"
REVIEW_WEB_URL="${REVIEW_WEB_URL:-http://127.0.0.1:${WEB_PORT}}"
REVIEW_OPENVIKING_URL="${REVIEW_OPENVIKING_URL:-http://127.0.0.1:${OPENVIKING_PORT}}"
REVIEW_PSQL_URL="${REVIEW_PSQL_URL:-postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@127.0.0.1:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable}"
OPENVIKING_NAMESPACE="${OPENVIKING_NAMESPACE:-aitask}"
AGENT_TOKEN_SECRET="${AGENT_TOKEN_SECRET:-dev_only_change_me_review}"
CONSOLE_OPERATOR_LABEL="${CONSOLE_OPERATOR_LABEL:-review-operator}"
AITASK_WORKER_ENABLED="${AITASK_WORKER_ENABLED:-true}"
AITASK_WORKER_START_DELAY="${AITASK_WORKER_START_DELAY:-1s}"
AITASK_WORKER_ACTIVE_RUN_TIMEOUT="${AITASK_WORKER_ACTIVE_RUN_TIMEOUT:-12s}"
AITASK_WORKER_ACTIVE_RUN_SWEEP_INTERVAL="${AITASK_WORKER_ACTIVE_RUN_SWEEP_INTERVAL:-2s}"
AITASK_WORKER_REVIEW_SWEEP_INTERVAL="${AITASK_WORKER_REVIEW_SWEEP_INTERVAL:-3s}"
AITASK_WORKER_PROGRESS_SWEEP_INTERVAL="${AITASK_WORKER_PROGRESS_SWEEP_INTERVAL:-3s}"
AITASK_WORKER_COMPLETION_SWEEP_INTERVAL="${AITASK_WORKER_COMPLETION_SWEEP_INTERVAL:-3s}"
AITASK_WORKER_PRESENCE_SWEEP_INTERVAL="${AITASK_WORKER_PRESENCE_SWEEP_INTERVAL:-2s}"
AITASK_WORKER_PRESENCE_TTL="${AITASK_WORKER_PRESENCE_TTL:-4s}"
AITASK_WORKER_TASK_SUMMARY_SWEEP_INTERVAL="${AITASK_WORKER_TASK_SUMMARY_SWEEP_INTERVAL:-3s}"
AITASK_WORKER_HANDOFF_SWEEP_INTERVAL="${AITASK_WORKER_HANDOFF_SWEEP_INTERVAL:-3s}"
AITASK_WORKER_BATCH_SIZE="${AITASK_WORKER_BATCH_SIZE:-500}"
AITASK_WORKER_DAILY_SUMMARY_CRON="${AITASK_WORKER_DAILY_SUMMARY_CRON:-@daily}"
BACKEND_DIR="$ROOT_DIR/backend"
FRONTED_DIR="$ROOT_DIR/web"
AITASK_CLI_BIN="${AITASK_CLI_BIN:-$TMP_ROOT/bin/aitask}"

mkdir -p "$ARTIFACTS_ROOT" "$RUNTIME_ROOT" "$PERF_ROOT" "$DIFF_ROOT" "$TMP_ROOT"

review_log() {
  printf '[review] %s\n' "$*"
}

review_fail() {
  printf '[review] FAIL %s\n' "$*" >&2
  exit 1
}

review_require() {
  local tool="$1"
  command -v "$tool" >/dev/null 2>&1 || review_fail "missing required tool: $tool"
}

review_slug() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9._-' '-'
}

review_jq() {
  local expr="$1"
  local file="$2"
  jq -r "$expr" "$file"
}

review_json_write() {
  local path="$1"
  local json="$2"
  mkdir -p "$(dirname "$path")"
  printf '%s\n' "$json" >"$path"
}

review_note() {
  local path="$1"
  shift
  mkdir -p "$(dirname "$path")"
  printf '%s\n' "$*" >>"$path"
}

review_write_env() {
  mkdir -p "$(dirname "$REVIEW_ENV_FILE")"
  cat >"$REVIEW_ENV_FILE" <<EOF
AITASK_ENV=development
APP_PORT=${APP_PORT}
WEB_PORT=${WEB_PORT}
POSTGRES_PORT=${POSTGRES_PORT}
DRAGONFLY_PORT=${DRAGONFLY_PORT}
OPENVIKING_PORT=${OPENVIKING_PORT}
POSTGRES_DB=${POSTGRES_DB}
POSTGRES_USER=${POSTGRES_USER}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}?sslmode=disable
DRAGONFLY_PASSWORD=${DRAGONFLY_PASSWORD}
DRAGONFLY_URL=redis://:${DRAGONFLY_PASSWORD}@dragonfly:6379/0
OPENVIKING_BASE_URL=http://openviking:9090
OPENVIKING_NAMESPACE=${OPENVIKING_NAMESPACE}
AGENT_TOKEN_SECRET=${AGENT_TOKEN_SECRET}
CONSOLE_OPERATOR_LABEL=${CONSOLE_OPERATOR_LABEL}
VITE_API_BASE_URL=${VITE_API_BASE_URL:-}
VITE_WS_BASE_URL=${VITE_WS_BASE_URL:-}
AITASK_WORKER_ENABLED=${AITASK_WORKER_ENABLED}
AITASK_WORKER_START_DELAY=${AITASK_WORKER_START_DELAY}
AITASK_WORKER_ACTIVE_RUN_TIMEOUT=${AITASK_WORKER_ACTIVE_RUN_TIMEOUT}
AITASK_WORKER_ACTIVE_RUN_SWEEP_INTERVAL=${AITASK_WORKER_ACTIVE_RUN_SWEEP_INTERVAL}
AITASK_WORKER_REVIEW_SWEEP_INTERVAL=${AITASK_WORKER_REVIEW_SWEEP_INTERVAL}
AITASK_WORKER_PROGRESS_SWEEP_INTERVAL=${AITASK_WORKER_PROGRESS_SWEEP_INTERVAL}
AITASK_WORKER_COMPLETION_SWEEP_INTERVAL=${AITASK_WORKER_COMPLETION_SWEEP_INTERVAL}
AITASK_WORKER_PRESENCE_SWEEP_INTERVAL=${AITASK_WORKER_PRESENCE_SWEEP_INTERVAL}
AITASK_WORKER_PRESENCE_TTL=${AITASK_WORKER_PRESENCE_TTL}
AITASK_WORKER_TASK_SUMMARY_SWEEP_INTERVAL=${AITASK_WORKER_TASK_SUMMARY_SWEEP_INTERVAL}
AITASK_WORKER_HANDOFF_SWEEP_INTERVAL=${AITASK_WORKER_HANDOFF_SWEEP_INTERVAL}
AITASK_WORKER_BATCH_SIZE=${AITASK_WORKER_BATCH_SIZE}
AITASK_WORKER_DAILY_SUMMARY_CRON=${AITASK_WORKER_DAILY_SUMMARY_CRON}
EOF
}

compose_cmd() {
  docker compose -p "$COMPOSE_PROJECT_NAME" -f "$COMPOSE_FILE" --env-file "$REVIEW_ENV_FILE" "$@"
}

review_compose_up() {
  review_write_env
  review_log "starting compose stack with project ${COMPOSE_PROJECT_NAME}"
  if [[ "$REVIEW_BUILD" == "1" ]]; then
    compose_cmd up -d --build
  else
    compose_cmd up -d
  fi
  ENV_FILE="$REVIEW_ENV_FILE" COMPOSE_FILE="$COMPOSE_FILE" COMPOSE_PROJECT_NAME="$COMPOSE_PROJECT_NAME" \
    bash "$ROOT_DIR/scripts/migrate.sh" up
}

review_compose_down() {
  review_log "stopping compose stack ${COMPOSE_PROJECT_NAME}"
  compose_cmd down -v --remove-orphans || true
}

review_wait_http_json() {
  local url="$1"
  local artifact="$2"
  local deadline=$((SECONDS + REVIEW_WAIT_SECONDS))
  while true; do
    if curl -fsS "$url" >"$artifact" 2>/dev/null; then
      return 0
    fi
    if ((SECONDS >= deadline)); then
      review_fail "timeout waiting for ${url}"
    fi
    sleep 1
  done
}

review_wait_ready() {
  local ready_json="$RUNTIME_ROOT/readyz.json"
  local deadline=$((SECONDS + REVIEW_WAIT_SECONDS))
  review_log "waiting for backend readiness ${REVIEW_SERVER_URL}/readyz"
  while true; do
    if curl -fsS "${REVIEW_SERVER_URL}/readyz" >"$ready_json" 2>/dev/null; then
      if jq -e '.status == "ready" or .status == "degraded"' "$ready_json" >/dev/null; then
        return 0
      fi
    fi
    if ((SECONDS >= deadline)); then
      compose_cmd ps >"$RUNTIME_ROOT/compose-ps.txt" || true
      compose_cmd logs >"$RUNTIME_ROOT/compose-logs.txt" || true
      review_fail "backend did not reach ready/degraded within ${REVIEW_WAIT_SECONDS}s"
    fi
    sleep 1
  done
}

review_collect_compose_state() {
  compose_cmd ps --format json >"$RUNTIME_ROOT/compose-ps.json" || true
  compose_cmd logs --no-color >"$RUNTIME_ROOT/compose-logs.txt" || true
}

review_http() {
  local method="$1"
  local path="$2"
  local body_file="${3:-}"
  local output="$4"
  local token="${5:-}"
  local url="${REVIEW_SERVER_URL}${path}"
  local args=(-sS -X "$method" -H 'Accept: application/json')
  if [[ -n "$token" ]]; then
    args+=(-H "Authorization: Bearer ${token}")
  fi
  if [[ -n "$body_file" ]]; then
    args+=(-H 'Content-Type: application/json' --data-binary "@${body_file}")
  fi
  curl "${args[@]}" "$url" >"$output"
}

review_http_status() {
  local method="$1"
  local path="$2"
  local body_file="${3:-}"
  local output="$4"
  local token="${5:-}"
  local url="${REVIEW_SERVER_URL}${path}"
  local args=(-sS -o "$output" -w '%{http_code}' -X "$method" -H 'Accept: application/json')
  if [[ -n "$token" ]]; then
    args+=(-H "Authorization: Bearer ${token}")
  fi
  if [[ -n "$body_file" ]]; then
    args+=(-H 'Content-Type: application/json' --data-binary "@${body_file}")
  fi
  curl "${args[@]}" "$url"
}

review_psql() {
  if command -v psql >/dev/null 2>&1; then
    PGPASSWORD="$POSTGRES_PASSWORD" psql "$REVIEW_PSQL_URL" "$@"
    return
  fi
  compose_cmd exec -T -e PGPASSWORD="$POSTGRES_PASSWORD" postgres \
    psql "postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@127.0.0.1:5432/${POSTGRES_DB}?sslmode=disable" "$@"
}

review_redis() {
  if command -v redis-cli >/dev/null 2>&1; then
    redis-cli -u "redis://:${DRAGONFLY_PASSWORD}@127.0.0.1:${DRAGONFLY_PORT}/0" "$@"
    return
  fi
  compose_cmd exec -T dragonfly redis-cli -a "$DRAGONFLY_PASSWORD" "$@"
}

review_assert_json() {
  local file="$1"
  local expr="$2"
  jq -e "$expr" "$file" >/dev/null || review_fail "assertion failed: jq ${expr} on ${file}"
}

review_make_home() {
  local name="$1"
  local home="$TMP_ROOT/home-$(review_slug "$name")"
  mkdir -p "$home"
  printf '%s\n' "$home"
}

review_make_workspace() {
  local name="$1"
  local dir="$TMP_ROOT/workspace-$(review_slug "$name")"
  rm -rf "$dir"
  mkdir -p "$dir"
  printf '%s\n' "$dir"
}

review_cli() {
  local home="$1"
  local workdir="$2"
  shift 2
  review_ensure_cli_bin
  (
    cd "$workdir"
    AITASK_TOKEN_STORE=file HOME="$home" "$AITASK_CLI_BIN" --server "$REVIEW_SERVER_URL" --format "$REVIEW_CLI_FORMAT" "$@"
  )
}

review_ensure_cli_bin() {
  if [[ -x "$AITASK_CLI_BIN" ]] && ! find "$BACKEND_DIR" -type f \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) -newer "$AITASK_CLI_BIN" -print -quit | grep -q .; then
    return 0
  fi
  mkdir -p "$(dirname "$AITASK_CLI_BIN")"
  (
    cd "$BACKEND_DIR"
    GOMODCACHE="${TMP_ROOT}/gomodcache" GOCACHE="${TMP_ROOT}/gocache" \
      go build -o "$AITASK_CLI_BIN" ./cmd/aitask
  )
}

review_cli_capture() {
  local home="$1"
  local workdir="$2"
  local output="$3"
  shift 3
  review_cli "$home" "$workdir" "$@" >"$output"
}

review_playwright() {
  local spec="$1"
  shift
  (
    cd "$FRONTED_DIR"
    PLAYWRIGHT_BASE_URL="$REVIEW_WEB_URL" PLAYWRIGHT_PORT="$WEB_PORT" pnpm exec playwright test "$spec" --config playwright.config.ts --reporter=line --workers=1 --timeout=30000 --grep-invert "home smoke" "$@"
  )
}

review_capture_health() {
  curl -fsS "${REVIEW_SERVER_URL}/healthz" >"$RUNTIME_ROOT/healthz.json"
  curl -fsS "${REVIEW_SERVER_URL}/readyz" >"$RUNTIME_ROOT/readyz.json"
}

review_teardown_trap() {
  if [[ "${REVIEW_KEEP_STACK:-0}" != "1" ]]; then
    trap review_compose_down EXIT
  fi
}

review_seed_json() {
  local path="$1"
  local json="$2"
  mkdir -p "$(dirname "$path")"
  printf '%s\n' "$json" >"$path"
}

review_start_watch() {
  local home="$1"
  local workdir="$2"
  local stdout_path="$3"
  local stderr_path="$4"
  local pid_path="$5"
  review_cli "$home" "$workdir" room watch >"$stdout_path" 2>"$stderr_path" &
  local pid=$!
  printf '%s\n' "$pid" >"$pid_path"
}

review_stop_watch() {
  local pid_path="$1"
  if [[ ! -f "$pid_path" ]]; then
    return 0
  fi
  local pid
  pid="$(cat "$pid_path")"
  if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
}

review_now() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}
