# syntax=docker/dockerfile:1.7
# migrate-init image: bundles golang-migrate CLI + AITask postgres migrations.
# Runs migrate up once, marks /tmp/done so the Lazycat healthcheck passes,
# then sleeps so the container stays in healthy state for downstream gating.
FROM golang:1.25-alpine AS builder
RUN go install -tags 'postgres' \
    github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.0

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /go/bin/migrate /usr/local/bin/migrate
COPY postgres /migrations
RUN cat > /entrypoint.sh <<'SH' && chmod +x /entrypoint.sh
#!/bin/sh
set -eu

if [ -z "${DATABASE_URL:-}" ]; then
  echo "[migrate-init] DATABASE_URL is required"
  exit 1
fi

MAX_RETRIES="${MIGRATE_MAX_RETRIES:-60}"
RETRY_INTERVAL_SECONDS="${MIGRATE_RETRY_INTERVAL_SECONDS:-2}"
attempt=1

is_transient_db_error() {
  echo "$1" | grep -Eqi \
    "connection refused|connection reset|i/o timeout|timed out|no route to host|temporary failure|the database system is starting up|server closed the connection unexpectedly"
}

echo "[migrate-init] applying migrations from /migrations against $DATABASE_URL"
while true; do
  set +e
  output="$(migrate -path=/migrations -database="$DATABASE_URL" up 2>&1)"
  status=$?
  set -e

  if [ "$status" -eq 0 ]; then
    [ -n "$output" ] && echo "$output"
    break
  fi

  [ -n "$output" ] && echo "$output"

  # golang-migrate may return non-zero with "no change"; treat as successful idempotent run.
  if echo "$output" | grep -qi "no change"; then
    echo "[migrate-init] no pending migrations"
    break
  fi

  if [ "$attempt" -lt "$MAX_RETRIES" ] && is_transient_db_error "$output"; then
    echo "[migrate-init] database not ready (attempt $attempt/$MAX_RETRIES), retrying in ${RETRY_INTERVAL_SECONDS}s..."
    attempt=$((attempt + 1))
    sleep "$RETRY_INTERVAL_SECONDS"
    continue
  fi

  echo "[migrate-init] migration failed permanently"
  exit "$status"
done

echo "[migrate-init] done"
touch /tmp/done
exec sleep infinity
SH
ENTRYPOINT ["/entrypoint.sh"]
