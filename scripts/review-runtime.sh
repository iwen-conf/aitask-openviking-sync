#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/scripts/lib/review-common.sh"

review_require jq
review_require curl
review_require docker
review_require pnpm

ARTIFACT_DIR="${ARTIFACT_DIR:-$RUNTIME_ROOT}"
SUMMARY_JSON="$ARTIFACT_DIR/summary.json"
SUMMARY_MD="$ARTIFACT_DIR/summary.md"
PROJECT_JSON="$ARTIFACT_DIR/project.json"
PROJECT_ID_FILE="$ARTIFACT_DIR/project-id.txt"
AGENTS_JSON="$ARTIFACT_DIR/agents.json"
TOKENS_DIR="$ARTIFACT_DIR/tokens"
CLI_DIR="$ARTIFACT_DIR/cli"
WS_DIR="$ARTIFACT_DIR/ws"
STATE_DIR="$ARTIFACT_DIR/state"
CODEX_TASK_FILE="$STATE_DIR/codex-task-id.txt"
REVIEW_TASK_FILE="$STATE_DIR/review-task-id.txt"
CLAUDE_HOME_FILE="$STATE_DIR/claude-home.txt"
CLAUDE_WORKDIR_FILE="$STATE_DIR/claude-workdir.txt"
CODEX_HOME_FILE="$STATE_DIR/codex-home.txt"
CODEX_WORKDIR_FILE="$STATE_DIR/codex-workdir.txt"
GEMINI_HOME_FILE="$STATE_DIR/gemini-home.txt"
GEMINI_WORKDIR_FILE="$STATE_DIR/gemini-workdir.txt"
CLAUDE_WATCH_PID_FILE="$STATE_DIR/claude-watch.pid"
CODEX_WATCH_PID_FILE="$STATE_DIR/codex-watch.pid"
GEMINI_WATCH_PID_FILE="$STATE_DIR/gemini-watch.pid"

mkdir -p "$ARTIFACT_DIR" "$TOKENS_DIR" "$CLI_DIR" "$WS_DIR" "$STATE_DIR"

review_teardown_trap

review_runtime_boot() {
  review_ensure_cli_bin
  review_compose_up
  review_wait_ready
  review_collect_compose_state
  review_capture_health
  curl -fsS "$REVIEW_WEB_URL" >"$ARTIFACT_DIR/web-home.html"
  review_assert_json "$ARTIFACT_DIR/healthz.json" '.status == "ok"'
  review_assert_json "$ARTIFACT_DIR/readyz.json" '.dependencies.postgres == "ok" and .dependencies.dragonfly == "ok"'
}

review_runtime_create_project() {
  REVIEW_PROJECT_NAME="RV Runtime Project $(date +%H%M%S)" \
  REVIEW_PROJECT_GOAL="Validate RV-001~009 and RV-020~029 in real runtime" \
  REVIEW_PROJECT_DESCRIPTION="Created by Playwright for review runtime evidence" \
    review_playwright "e2e/review-runtime.spec.ts" \
    --reporter=line >"$ARTIFACT_DIR/playwright.log"

  review_http GET "/api/projects" "" "$ARTIFACT_DIR/projects-list.json"
  jq -e '.items | length > 0' "$ARTIFACT_DIR/projects-list.json" >/dev/null || review_fail "no projects returned after web create"
  jq '.items[0]' "$ARTIFACT_DIR/projects-list.json" >"$PROJECT_JSON"
  review_jq '.projectId' "$PROJECT_JSON" >"$PROJECT_ID_FILE"
  PROJECT_ID="$(cat "$PROJECT_ID_FILE")"
  review_http GET "/api/projects/${PROJECT_ID}" "" "$ARTIFACT_DIR/project-detail.json"
  review_assert_json "$ARTIFACT_DIR/project-detail.json" '.projectId != null and .roomId != null and .openvikingRoot != null'
  printf '{"completionPolicy":{"requiredTasks":"optional","blockedTasks":"allow","failedTasks":"allow","reviewPolicy":"optional"}}\n' >"$ARTIFACT_DIR/project-policy-request.json"
  review_http PATCH "/api/projects/${PROJECT_ID}" "$ARTIFACT_DIR/project-policy-request.json" "$ARTIFACT_DIR/project-policy.json"
  review_http GET "/api/projects/${PROJECT_ID}/memory" "" "$ARTIFACT_DIR/project-memory.json"
  review_http GET "/api/projects/${PROJECT_ID}/room" "" "$ARTIFACT_DIR/project-room.json"
  review_assert_json "$ARTIFACT_DIR/project-memory.json" '.items | length >= 10'
  review_assert_json "$ARTIFACT_DIR/project-room.json" '.roomId != null and .projectId != null'
  review_psql -Atc "select id, active_session_id, openviking_root_uri from projects where id = '${PROJECT_ID}'" >"$ARTIFACT_DIR/project-row.txt"
}

review_runtime_openviking_tree() {
  PROJECT_ID="$(cat "$PROJECT_ID_FILE")"
  docker run --rm \
    -v "${COMPOSE_PROJECT_NAME}_openviking_data:/data:ro" \
    busybox:1.37.0 \
    find "/data/namespaces/aitask/projects/${PROJECT_ID}" -maxdepth 3 \
    | sort >"$ARTIFACT_DIR/openviking-tree.txt"
  for expected in \
    "brief" \
    "memory/decisions" \
    "memory/summaries" \
    "memory/agent-experience" \
    "memory/room" \
    "memory/handoffs" \
    "memory/mistakes" \
    "memory/notes" \
    "memory/reports" \
    "resources/api" \
    "resources/database" \
    "resources/cli" \
    "resources/frontend" \
    "skills" \
    "tasks" \
    "sessions"; do
    grep -F "/${expected}" "$ARTIFACT_DIR/openviking-tree.txt" >/dev/null || review_fail "missing openviking scaffold path ${expected}"
  done
}

review_runtime_agents_and_tokens() {
  review_http GET "/api/agents" "" "$AGENTS_JSON"
  for agent_type in "claude-code" "codex" "gemini"; do
    local agent_json="$ARTIFACT_DIR/agent-${agent_type}.json"
    jq --arg type "$agent_type" '.items[] | select(.agentType == $type)' "$AGENTS_JSON" >"$agent_json"
    jq -e '.agentId != null' "$agent_json" >/dev/null || review_fail "default agent ${agent_type} missing"
    local agent_id
    agent_id="$(review_jq '.agentId' "$agent_json")"
    local token_req="$TOKENS_DIR/${agent_type}-request.json"
    local token_res="$TOKENS_DIR/${agent_type}.json"
    jq -n --arg expires "$(date -u -v+1d +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || python - <<'PY'
from datetime import datetime, timedelta, timezone
print((datetime.now(timezone.utc)+timedelta(days=1)).strftime("%Y-%m-%dT%H:%M:%SZ"))
PY
)" --argjson scopes "$(jq '.scopes' "$agent_json")" '{expiresAt:$expires,scopes:$scopes}' >"$token_req"
    review_http POST "/api/agents/${agent_id}/tokens" "$token_req" "$token_res"
    jq -e '.agentToken != null' "$token_res" >/dev/null || review_fail "token issuance failed for ${agent_type}"
    local bind_req="$TOKENS_DIR/${agent_type}-bind-request.json"
    printf '{"role":"%s","enabled":true}\n' "$(review_jq '.role' "$agent_json")" >"$bind_req"
    review_http POST "/api/projects/$(cat "$PROJECT_ID_FILE")/agents/${agent_id}/bind" "$bind_req" "$TOKENS_DIR/${agent_type}-bind.json"
  done
}

review_runtime_cli_flow() {
  PROJECT_ID="$(cat "$PROJECT_ID_FILE")"
  local claude_home codex_home gemini_home

  claude_home="$(review_make_home claude-code)"
  codex_home="$(review_make_home codex)"
  gemini_home="$(review_make_home gemini)"
  local claude_ws_workdir codex_ws_workdir gemini_ws_workdir
  claude_ws_workdir="$(review_make_workspace claude-code)"
  codex_ws_workdir="$(review_make_workspace codex)"
  gemini_ws_workdir="$(review_make_workspace gemini)"
  printf '%s\n' "$claude_home" >"$CLAUDE_HOME_FILE"
  printf '%s\n' "$claude_ws_workdir" >"$CLAUDE_WORKDIR_FILE"
  printf '%s\n' "$codex_home" >"$CODEX_HOME_FILE"
  printf '%s\n' "$codex_ws_workdir" >"$CODEX_WORKDIR_FILE"
  printf '%s\n' "$gemini_home" >"$GEMINI_HOME_FILE"
  printf '%s\n' "$gemini_ws_workdir" >"$GEMINI_WORKDIR_FILE"

  review_cli_capture "$claude_home" "$claude_ws_workdir" "$CLI_DIR/claude-auth.json" auth token import --token "$(review_jq '.agentToken' "$TOKENS_DIR/claude-code.json")"
  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-auth.json" auth token import --token "$(review_jq '.agentToken' "$TOKENS_DIR/codex.json")"
  review_cli_capture "$gemini_home" "$gemini_ws_workdir" "$CLI_DIR/gemini-auth.json" auth token import --token "$(review_jq '.agentToken' "$TOKENS_DIR/gemini.json")"

  review_cli_capture "$claude_home" "$claude_ws_workdir" "$CLI_DIR/claude-init.json" init --project "$PROJECT_ID"
  review_cli_capture "$claude_home" "$claude_ws_workdir" "$CLI_DIR/claude-project-info.json" project info
  review_cli_capture "$claude_home" "$claude_ws_workdir" "$CLI_DIR/claude-skill-pull.json" skill pull
  review_cli_capture "$claude_home" "$claude_ws_workdir" "$CLI_DIR/claude-room-join.json" room join
  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-init.json" init --project "$PROJECT_ID"
  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-room-join.json" room join
  review_cli_capture "$gemini_home" "$gemini_ws_workdir" "$CLI_DIR/gemini-init.json" init --project "$PROJECT_ID"
  review_cli_capture "$gemini_home" "$gemini_ws_workdir" "$CLI_DIR/gemini-room-join.json" room join

  test -f "$claude_ws_workdir/.aitask/project.md" || review_fail "aitask init did not create project.md"
  test -f "$codex_ws_workdir/.aitask/project.md" || review_fail "codex init did not create project.md"
  test -f "$gemini_ws_workdir/.aitask/project.md" || review_fail "gemini init did not create project.md"

  review_start_watch "$claude_home" "$claude_ws_workdir" "$WS_DIR/claude.ndjson" "$WS_DIR/claude.err" "$CLAUDE_WATCH_PID_FILE"
  review_start_watch "$codex_home" "$codex_ws_workdir" "$WS_DIR/codex.ndjson" "$WS_DIR/codex.err" "$CODEX_WATCH_PID_FILE"
  review_start_watch "$gemini_home" "$gemini_ws_workdir" "$WS_DIR/gemini.ndjson" "$WS_DIR/gemini.err" "$GEMINI_WATCH_PID_FILE"
  sleep 2

  local create_codex="$CLI_DIR/create-codex-task.json"
  local create_gemini="$CLI_DIR/create-gemini-task.json"
  review_cli_capture "$claude_home" "$claude_ws_workdir" "$create_codex" task create --title "RV codex task" --description "Implement runtime worker flow" --target codex --skill backend-implementation
  review_cli_capture "$claude_home" "$claude_ws_workdir" "$create_gemini" task create --title "RV gemini task" --description "Prepare runtime evidence notes" --target gemini --skill document-generation

  local codex_task gemini_task codex_run
  codex_task="$(review_jq '.taskId' "$create_codex")"
  gemini_task="$(review_jq '.taskId' "$create_gemini")"
  printf '%s\n' "$codex_task" >"$CODEX_TASK_FILE"

  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-bootstrap.json" bootstrap
  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-current.json" task current
  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-start.json" task start "$codex_task" --run run_rv_codex_001
  codex_run="$(review_jq '.activeRunId' "$CLI_DIR/codex-start.json")"
  review_seed_json "$codex_ws_workdir/.aitask/result.md" "# Runtime Review Result\n\nImplemented runtime validation path.\n"
  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-heartbeat.json" task heartbeat --task "$codex_task" --run "$codex_run"
  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-context-report.json" context report --input 170000 --output 20000 --max 200000 --run "$codex_run" --source codex
  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-handoff-prepare.json" context handoff prepare
  cat >"$codex_ws_workdir/.aitask/handoff.md" <<EOF
# Agent Handoff

- task_id: ${codex_task}
- run_id: ${codex_run}
- summary: runtime review handoff prepared
EOF
  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-handoff-submit.json" context handoff submit --task "$codex_task" --reason context_limit_handoff --from "$codex_ws_workdir/.aitask/handoff.md"
  local handoff_id
  handoff_id="$(review_jq '.handoffId' "$CLI_DIR/codex-handoff-submit.json")"
  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-resume.json" task resume "$codex_task" --handoff "$handoff_id" --run run_rv_codex_002
  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-current-handoff.json" context handoff current || true
  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-room-send.json" room send "Codex resumed and continuing review runtime flow"
  review_cli_capture "$codex_home" "$codex_ws_workdir" "$CLI_DIR/codex-submit.json" task submit "$codex_task" --from "$codex_ws_workdir/.aitask/result.md" --run run_rv_codex_002

  review_cli_capture "$gemini_home" "$gemini_ws_workdir" "$CLI_DIR/gemini-current.json" task current
  review_http_status GET "/api/projects/${PROJECT_ID}/tasks?assigneeAgentId=$(review_jq '.agentId' "$ARTIFACT_DIR/agent-gemini.json")" "" "$CLI_DIR/gemini-own-status-check.json" "$(review_jq '.agentToken' "$TOKENS_DIR/gemini.json")" >"$CLI_DIR/gemini-own-status.status"
  review_http_status GET "/api/projects/${PROJECT_ID}/tasks?assigneeAgentType=codex" "" "$CLI_DIR/gemini-cross-read.json" "$(review_jq '.agentToken' "$TOKENS_DIR/gemini.json")" >"$CLI_DIR/gemini-cross-read.status"

  review_http GET "/api/projects/${PROJECT_ID}/tasks?status=delegated&assigneeAgentType=claude-code" "" "$CLI_DIR/review-tasks.json" "$(review_jq '.agentToken' "$TOKENS_DIR/claude-code.json")"
  local review_task
  review_task="$(jq -r --arg parent "$codex_task" '.items[] | select(.parentTaskId == $parent) | .taskId' "$CLI_DIR/review-tasks.json" | head -n1)"
  [[ -n "$review_task" && "$review_task" != "null" ]] || review_fail "review task was not created for ${codex_task}"
  printf '%s\n' "$review_task" >"$REVIEW_TASK_FILE"

  review_cli_capture "$claude_home" "$claude_ws_workdir" "$CLI_DIR/claude-review-current.json" task current
  review_cli_capture "$claude_home" "$claude_ws_workdir" "$CLI_DIR/claude-review-start.json" task start "$review_task" --run run_rv_claude_review_001
  [[ "$(cat "$CLI_DIR/gemini-own-status.status")" == "200" ]] || review_fail "expected gemini own task list to return 200"
  [[ "$(cat "$CLI_DIR/gemini-cross-read.status")" == "403" ]] || review_fail "expected gemini cross-agent task read to return 403"
}

review_runtime_degraded_checks() {
  local project_id
  project_id="$(cat "$PROJECT_ID_FILE")"
  curl -fsS "${REVIEW_SERVER_URL}/readyz" >"$ARTIFACT_DIR/readyz-before-degraded.json"

  compose_cmd stop openviking
  sleep 3
  curl -fsS "${REVIEW_SERVER_URL}/readyz" >"$ARTIFACT_DIR/readyz-openviking-down.json"
  review_assert_json "$ARTIFACT_DIR/readyz-openviking-down.json" '.status == "degraded" and .dependencies.openviking == "unavailable"'
  review_http GET "/api/projects/${project_id}/tasks?status=submitted" "" "$CLI_DIR/submitted-while-openviking-down.json" "$(review_jq '.agentToken' "$TOKENS_DIR/claude-code.json")"

  local create_status
  printf '{"name":"ov down","goal":"must fail","description":"openviking unavailable"}\n' >"$ARTIFACT_DIR/openviking-down-create-request.json"
  create_status="$(review_http_status POST "/api/projects" "$ARTIFACT_DIR/openviking-down-create-request.json" "$ARTIFACT_DIR/openviking-down-create.json")"
  printf '%s\n' "$create_status" >"$ARTIFACT_DIR/openviking-down-create.status"
  [[ "$create_status" =~ ^5 ]] || review_fail "expected create project with openviking down to fail 5xx, got ${create_status}"

  compose_cmd start openviking
  sleep 4
  review_wait_ready

  compose_cmd stop dragonfly
  sleep 3
  local dragonfly_status
  dragonfly_status="$(curl -sS -o "$ARTIFACT_DIR/readyz-dragonfly-down.json" -w '%{http_code}' "${REVIEW_SERVER_URL}/readyz")"
  printf '%s\n' "$dragonfly_status" >"$ARTIFACT_DIR/readyz-dragonfly-down.status"
  [[ "$dragonfly_status" == "503" ]] || review_fail "expected readyz 503 with dragonfly down, got ${dragonfly_status}"
  review_assert_json "$ARTIFACT_DIR/readyz-dragonfly-down.json" '.status == "not_ready" and .dependencies.dragonfly == "unavailable"'

  compose_cmd start dragonfly
  sleep 4
  review_wait_ready
}

review_runtime_complete_and_archive() {
  local project_id review_task claude_home claude_workdir archived_status
  project_id="$(cat "$PROJECT_ID_FILE")"
  review_task="$(cat "$REVIEW_TASK_FILE")"
  claude_home="$(cat "$CLAUDE_HOME_FILE")"
  claude_workdir="$(cat "$CLAUDE_WORKDIR_FILE")"

  review_cli_capture "$claude_home" "$claude_workdir" "$CLI_DIR/claude-review.json" task review "$review_task" --approve --comment "runtime flow approved"
  printf '{"confirm":true}\n' >"$CLI_DIR/project-complete-request.json"
  review_http POST "/api/projects/${project_id}/complete" "$CLI_DIR/project-complete-request.json" "$CLI_DIR/project-complete.json"
  printf '{"confirm":true,"reason":"runtime review complete"}\n' >"$CLI_DIR/project-archive-request.json"
  review_http POST "/api/projects/${project_id}/archive" "$CLI_DIR/project-archive-request.json" "$CLI_DIR/project-archive.json"
  review_assert_json "$CLI_DIR/project-archive.json" '.status == "archived"'

  printf '{"messageType":"text","content":"should fail"}\n' >"$CLI_DIR/archive-room-write-request.json"
  archived_status="$(review_http_status POST "/api/projects/${project_id}/room/messages" "$CLI_DIR/archive-room-write-request.json" "$CLI_DIR/archive-room-write.json")"
  printf '%s\n' "$archived_status" >"$CLI_DIR/archive-room-write.status"
  [[ "$archived_status" == "409" ]] || review_fail "expected archived room write to return 409, got ${archived_status}"
  review_assert_json "$CLI_DIR/archive-room-write.json" '.code == "PROJECT_ACCESS_DENIED" and .details.status == "archived"'
}

review_runtime_shutdown_watchers() {
  review_stop_watch "$CLAUDE_WATCH_PID_FILE"
  review_stop_watch "$CODEX_WATCH_PID_FILE"
  review_stop_watch "$GEMINI_WATCH_PID_FILE"
}

review_runtime_finalize() {
  local project_id codex_task
  project_id="$(cat "$PROJECT_ID_FILE")"
  codex_task="$(cat "$CODEX_TASK_FILE")"
  review_http GET "/api/projects/${project_id}/tasks/${codex_task}/events" "" "$ARTIFACT_DIR/task-events.json"
  review_http GET "/api/projects/${project_id}/room/messages?limit=200" "" "$ARTIFACT_DIR/final-room-history.json"
  review_http GET "/api/projects/${project_id}/memory/search?q=runtime&budget=5000" "" "$ARTIFACT_DIR/final-memory-search.json"
  review_psql -Atc "select status from projects where id='${project_id}'" >"$ARTIFACT_DIR/final-project-status.txt"
  review_psql -Atc "select count(*) from project_room_messages where project_id='${project_id}'" >"$ARTIFACT_DIR/final-room-message-count.txt"
  review_redis --raw keys "room:presence:*" >"$ARTIFACT_DIR/dragonfly-presence-keys.txt"

  jq -n \
    --arg projectId "$project_id" \
    --arg status "$(cat "$ARTIFACT_DIR/final-project-status.txt")" \
    --arg readyStatus "$(jq -r '.status' "$ARTIFACT_DIR/readyz.json")" \
    --arg roomMessages "$(cat "$ARTIFACT_DIR/final-room-message-count.txt")" \
    '{
      runtime: {
        projectId: $projectId,
        finalProjectStatus: $status,
        initialReadyStatus: $readyStatus,
        roomMessageCount: ($roomMessages | tonumber),
        checks: [
          "RV-001~009",
          "RV-020~029"
        ]
      }
    }' >"$SUMMARY_JSON"

  cat >"$SUMMARY_MD" <<EOF
# Runtime Review Summary

- project_id: \`${project_id}\`
- final_project_status: \`$(cat "$ARTIFACT_DIR/final-project-status.txt")\`
- initial_readyz: \`$(jq -r '.status' "$ARTIFACT_DIR/readyz.json")\`
- room_messages: \`$(cat "$ARTIFACT_DIR/final-room-message-count.txt")\`
- archived_write_guard: PASS
- openviking_degraded: PASS
- dragonfly_not_ready: PASS
EOF
}

review_runtime_boot
review_runtime_create_project
review_runtime_openviking_tree
review_runtime_agents_and_tokens
review_runtime_cli_flow
review_runtime_degraded_checks
review_runtime_complete_and_archive
review_runtime_shutdown_watchers
review_runtime_finalize

review_log "runtime evidence written to ${ARTIFACT_DIR}"
