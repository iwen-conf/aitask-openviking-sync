#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/scripts/lib/review-common.sh"

review_require jq
review_require curl
review_require docker
review_require go

ARTIFACT_DIR="${ARTIFACT_DIR:-$PERF_ROOT}"
SUMMARY_JSON="$ARTIFACT_DIR/summary.json"
SUMMARY_MD="$ARTIFACT_DIR/summary.md"
STATE_DIR="$ARTIFACT_DIR/state"
TOKENS_DIR="$ARTIFACT_DIR/tokens"

mkdir -p "$ARTIFACT_DIR" "$STATE_DIR" "$TOKENS_DIR"

PERF_AGENT_JSON="$STATE_DIR/perf-agent.json"
PERF_TOKEN_JSON="$TOKENS_DIR/perf-agent.json"
PERF_PROJECT_JSON="$STATE_DIR/perf-project.json"
PERF_PROJECT_ID_FILE="$STATE_DIR/perf-project-id.txt"

review_perf_boot() {
  review_ensure_cli_bin
  review_compose_up
  review_wait_ready
  review_capture_health
}

review_perf_prepare_agent() {
  review_http GET "/api/agents" "" "$STATE_DIR/agents.json"
  jq '.items[] | select(.agentType == "codex")' "$STATE_DIR/agents.json" >"$PERF_AGENT_JSON"
  local agent_id
  agent_id="$(review_jq '.agentId' "$PERF_AGENT_JSON")"
  jq -n --arg expires "$(review_now)" --argjson scopes "$(jq '.scopes + ["room:read"]' "$PERF_AGENT_JSON")" '{expiresAt:$expires,scopes:$scopes}' >"$TOKENS_DIR/perf-agent-request.json"
  review_http POST "/api/agents/${agent_id}/tokens" "$TOKENS_DIR/perf-agent-request.json" "$PERF_TOKEN_JSON"
}

review_perf_prepare_project() {
  review_http GET "/api/projects" "" "$STATE_DIR/projects.json"
  if jq -e '.items | length > 0' "$STATE_DIR/projects.json" >/dev/null; then
    jq '.items[0]' "$STATE_DIR/projects.json" >"$PERF_PROJECT_JSON"
  else
    printf '{"name":"Perf Review","goal":"Collect performance evidence","description":"seeded project for RV-040~045"}\n' >"$STATE_DIR/perf-project-request.json"
    review_http POST "/api/projects" "$STATE_DIR/perf-project-request.json" "$PERF_PROJECT_JSON"
  fi
  review_jq '.projectId' "$PERF_PROJECT_JSON" >"$PERF_PROJECT_ID_FILE"
}

review_perf_seed_db() {
  local project_id agent_id
  project_id="$(cat "$PERF_PROJECT_ID_FILE")"
  agent_id="$(review_jq '.agentId' "$PERF_AGENT_JSON")"
  cat >"$STATE_DIR/perf-seed.sql" <<SQL
DO \$\$
DECLARE
  idx INT;
BEGIN
  FOR idx IN 1..5000 LOOP
    INSERT INTO tasks (
      id, project_id, session_id, title, description, status,
      assignee_agent_id, assignee_agent_type, delegated_by_type,
      priority, created_by_type, is_required, created_at, updated_at
    )
    VALUES (
      format('task_perf_%s', idx),
      '${project_id}',
      (SELECT active_session_id FROM projects WHERE id = '${project_id}'),
      format('Perf Task %s', idx),
      'performance seed',
      CASE WHEN idx = 1 THEN 'delegated' ELSE 'done' END,
      '${agent_id}',
      'codex',
      'system',
      0,
      'system',
      TRUE,
      NOW() - (idx || ' seconds')::interval,
      NOW() - (idx || ' seconds')::interval
    )
    ON CONFLICT (id) DO NOTHING;
  END LOOP;

  FOR idx IN 1..20000 LOOP
    INSERT INTO project_room_messages (
      id, room_id, project_id, sender_type, sender_agent_id, sender_agent_type,
      message_type, content, payload, created_at
    )
    VALUES (
      format('msg_perf_%s', idx),
      (SELECT id FROM project_rooms WHERE project_id = '${project_id}'),
      '${project_id}',
      'agent',
      '${agent_id}',
      'codex',
      'text',
      format('performance message %s', idx),
      '{}'::jsonb,
      NOW() - (idx || ' milliseconds')::interval
    )
    ON CONFLICT (id) DO NOTHING;
  END LOOP;

  FOR idx IN 1..200 LOOP
    INSERT INTO agent_runs (
      id, agent_id, project_id, session_id, status, model_name,
      max_context_tokens, estimated_used_tokens, context_state,
      started_at, last_heartbeat_at
    )
    VALUES (
      format('run_perf_stale_%s', idx),
      '${agent_id}',
      '${project_id}',
      (SELECT active_session_id FROM projects WHERE id = '${project_id}'),
      'active',
      'gpt-5-codex',
      200000,
      1000,
      'normal',
      NOW() - interval '1 hour',
      NOW() - interval '1 hour'
    )
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO tasks (
      id, project_id, session_id, title, description, status, assignee_agent_id,
      assignee_agent_type, delegated_by_type, active_run_id, created_by_type,
      is_required, created_at, updated_at, last_heartbeat_at
    )
    VALUES (
      format('task_perf_stale_%s', idx),
      '${project_id}',
      (SELECT active_session_id FROM projects WHERE id = '${project_id}'),
      format('Stale Task %s', idx),
      'stale worker task',
      'running',
      '${agent_id}',
      'codex',
      'system',
      format('run_perf_stale_%s', idx),
      'system',
      TRUE,
      NOW() - interval '1 hour',
      NOW() - interval '1 hour',
      NOW() - interval '1 hour'
    )
    ON CONFLICT (id) DO NOTHING;
  END LOOP;
END \$\$;
SQL
  review_psql -f "$STATE_DIR/perf-seed.sql" >"$ARTIFACT_DIR/perf-seed.log"

  for i in $(seq 1 500); do
    review_redis SADD "room:presence:${project_id}:online" "agent:perf-${i}" >/dev/null
    review_redis HSET "room:presence:${project_id}:connections" "agent:perf-${i}" 0 >/dev/null
  done
}

review_perf_rpc() {
  local project_id token
  project_id="$(cat "$PERF_PROJECT_ID_FILE")"
  token="$(review_jq '.agentToken' "$PERF_TOKEN_JSON")"
  (
    cd "$BACKEND_DIR"
    go run ./cmd/review-perf-helper rpc-p99 \
      --server "$REVIEW_SERVER_URL" \
      --project "$project_id" \
      --token "$token" \
      --concurrency 20 \
      --requests 200
  ) >"$ARTIFACT_DIR/rv-040-rpc.json"
}

review_perf_ws() {
  local project_id token
  project_id="$(cat "$PERF_PROJECT_ID_FILE")"
  token="$(review_jq '.agentToken' "$PERF_TOKEN_JSON")"
  (
    cd "$BACKEND_DIR"
    go run ./cmd/review-perf-helper ws-p99 \
      --server "$REVIEW_SERVER_URL" \
      --project "$project_id" \
      --token "$token" \
      --connections 100
  ) >"$ARTIFACT_DIR/rv-041-ws.json"
}

review_perf_room_history() {
  local project_id token url
  project_id="$(cat "$PERF_PROJECT_ID_FILE")"
  token="$(review_jq '.agentToken' "$PERF_TOKEN_JSON")"
  url="${REVIEW_SERVER_URL}/api/projects/${project_id}/room/messages?limit=200"
  for i in $(seq 1 20); do
    local start end ms
    start="$(python3 - <<'PY'
import time
print(time.time())
PY
)"
    curl -fsS -H "Authorization: Bearer ${token}" "$url" >"$ARTIFACT_DIR/room-history-${i}.json"
    end="$(python3 - <<'PY'
import time
print(time.time())
PY
)"
    ms="$(python3 - "$start" "$end" <<'PY'
import sys
start = float(sys.argv[1])
end = float(sys.argv[2])
print((end - start) * 1000)
PY
)"
    printf '%s\n' "$ms" >>"$ARTIFACT_DIR/rv-042-room-history-samples.txt"
  done
  python3 - "$ARTIFACT_DIR/rv-042-room-history-samples.txt" >"$ARTIFACT_DIR/rv-042-room-history.json" <<'PY'
import json, pathlib, statistics, sys
samples = sorted(float(line.strip()) for line in pathlib.Path(sys.argv[1]).read_text().splitlines() if line.strip())
def pct(p):
    if not samples:
        return None
    idx = min(len(samples)-1, int((len(samples)-1) * p))
    return samples[idx]
print(json.dumps({
    "kind": "room-history",
    "count": len(samples),
    "avgMs": statistics.mean(samples) if samples else None,
    "p95Ms": pct(0.95),
    "p99Ms": pct(0.99),
    "samples": samples,
}, indent=2))
PY
}

review_perf_explain() {
  local project_id agent_id
  project_id="$(cat "$PERF_PROJECT_ID_FILE")"
  agent_id="$(review_jq '.agentId' "$PERF_AGENT_JSON")"
  review_psql -Atc "EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) SELECT id, project_id, title, status FROM tasks WHERE project_id='${project_id}' AND assignee_agent_id='${agent_id}' AND status='delegated' ORDER BY created_at DESC, id DESC LIMIT 1;" >"$ARTIFACT_DIR/rv-043-current-task.explain.txt"
  review_psql -Atc "EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) SELECT id, room_id, project_id, message_type, created_at FROM project_room_messages WHERE project_id='${project_id}' ORDER BY created_at DESC, id DESC LIMIT 200;" >"$ARTIFACT_DIR/rv-043-room-history.explain.txt"
}

review_perf_worker_compare() {
  local project_id token
  project_id="$(cat "$PERF_PROJECT_ID_FILE")"
  token="$(review_jq '.agentToken' "$PERF_TOKEN_JSON")"

  (
    cd "$BACKEND_DIR"
    go run ./cmd/review-perf-helper rpc-p99 \
      --server "$REVIEW_SERVER_URL" \
      --project "$project_id" \
      --token "$token" \
      --concurrency 10 \
      --requests 100
  ) >"$ARTIFACT_DIR/rv-044-worker-on.json"

  review_compose_down
  AITASK_WORKER_ENABLED=false review_compose_up
  review_wait_ready

  (
    cd "$BACKEND_DIR"
    go run ./cmd/review-perf-helper rpc-p99 \
      --server "$REVIEW_SERVER_URL" \
      --project "$project_id" \
      --token "$token" \
      --concurrency 10 \
      --requests 100
  ) >"$ARTIFACT_DIR/rv-044-worker-off.json"

  review_compose_down
  AITASK_WORKER_ENABLED=true review_compose_up
  review_wait_ready
}

review_perf_dragonfly() {
  review_redis INFO MEMORY >"$ARTIFACT_DIR/rv-045-dragonfly-memory.txt"
  review_redis DBSIZE >"$ARTIFACT_DIR/rv-045-dragonfly-dbsize.txt"
}

review_perf_summary() {
  jq -n \
    --slurpfile rv040 "$ARTIFACT_DIR/rv-040-rpc.json" \
    --slurpfile rv041 "$ARTIFACT_DIR/rv-041-ws.json" \
    --slurpfile rv042 "$ARTIFACT_DIR/rv-042-room-history.json" \
    '{
      perf: {
        rv040: $rv040[0],
        rv041: $rv041[0],
        rv042: $rv042[0],
        rv043: {
          currentTaskExplain: "rv-043-current-task.explain.txt",
          roomHistoryExplain: "rv-043-room-history.explain.txt"
        },
        rv044: {
          workerOn: "rv-044-worker-on.json",
          workerOff: "rv-044-worker-off.json"
        },
        rv045: {
          dragonflyInfo: "rv-045-dragonfly-memory.txt",
          dbsize: "rv-045-dragonfly-dbsize.txt"
        }
      }
    }' >"$SUMMARY_JSON"

  cat >"$SUMMARY_MD" <<EOF
# Perf Review Summary

- RV-040 current delegated task query evidence: \`rv-040-rpc.json\`
- RV-041 websocket concurrency evidence: \`rv-041-ws.json\`
- RV-042 room history pagination evidence: \`rv-042-room-history.json\`
- RV-043 explain analyze evidence: \`rv-043-current-task.explain.txt\`, \`rv-043-room-history.explain.txt\`
- RV-044 worker on/off comparison: \`rv-044-worker-on.json\`, \`rv-044-worker-off.json\`
- RV-045 dragonfly memory evidence: \`rv-045-dragonfly-memory.txt\`
EOF
}

review_perf_boot
review_perf_prepare_agent
review_perf_prepare_project
review_perf_seed_db
review_perf_rpc
review_perf_ws
review_perf_room_history
review_perf_explain
review_perf_worker_compare
review_perf_dragonfly
review_perf_summary

review_log "perf evidence written to ${ARTIFACT_DIR}"
