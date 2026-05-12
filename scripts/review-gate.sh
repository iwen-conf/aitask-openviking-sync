#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

RUN_RUNTIME="${RUN_RUNTIME:-0}"
RUN_PERF="${RUN_PERF:-0}"
RUN_DIFF="${RUN_DIFF:-0}"

pass() { printf '[review-gate] PASS %s\n' "$1"; }
fail() { printf '[review-gate] FAIL %s\n' "$1" >&2; exit 1; }
contains() {
  local pattern="$1"; shift
  rg -q -- "$pattern" "$@" || fail "$pattern not found in $*"
}
contains_fixed() {
  local pattern="$1"; shift
  rg -F -q -- "$pattern" "$@" || fail "$pattern not found in $*"
}
not_contains() {
  local pattern="$1"; shift
  if rg -q -- "$pattern" "$@"; then
    fail "$pattern unexpectedly found in $*"
  fi
}

# RV-010: no CLI --agent identity override.
if (cd cli && go run ./aitask --help) | rg -q -- '--agent'; then
  fail 'RV-010 root help exposes --agent identity override'
fi
contains 'TestNormalizeCreateInputDropsDelegateType' core/internal/service/tasks/redline_test.go
contains 'DelegateToAgentType != ""' core/internal/service/tasks/redline_test.go
pass 'RV-010 no --agent flag and body agentType normalization is covered'

# RV-011: room messages are persisted as messages and pin creates memory_note, not task status changes.
contains 'func \(s \*Service\) SendMessage' core/internal/service/room/service.go
not_contains 'StatusDone|StatusSubmitted|StatusReviewing|UPDATE tasks' core/internal/service/room/service.go
pass 'RV-011 room service has no task completion write path'

# RV-012: OpenViking write target whitelist blocks authority fields.
contains 'TestClientWriteRejectsDisallowedTarget' core/internal/service/openviking/client_test.go
contains 'task_status' core/internal/service/openviking/client_test.go
pass 'RV-012 OpenViking authority-field rejection has unit coverage'

# RV-013: local project protocol warns against token storage and exposes token scanner.
contains 'ProjectFileContainsToken' cli/internal/cli/state.go
contains 'never store agent token' cli/internal/cli/state.go
pass 'RV-013 project.md token redline scanner/template present'

# RV-014: prompt is the default output format and JSON is explicit.
contains 'formatRaw: string\(FormatPrompt\)' cli/internal/cli/app.go
contains 'output format: brief\|prompt\|json\|proto' cli/internal/cli/app.go
pass 'RV-014 CLI defaults to prompt output'

# RV-015: one room per project by database uniqueness and route service.
contains 'project_id TEXT NOT NULL UNIQUE' migrations/postgres/000001_init.up.sql
contains 'ON CONFLICT \(project_id\) DO NOTHING' core/internal/service/room/service.go
pass 'RV-015 one project room uniqueness is enforced'

# RV-016/RV-017/RV-018: execution ownership, project isolation, and handoff-only gates have unit/API evidence.
contains 'TestSubmitRequiresMatchingActiveRun' core/internal/service/tasks/model_test.go
contains 'ErrTaskActiveRunMismatch' core/internal/service/tasks/model.go core/internal/service/tasks/model_test.go
contains 'TestRunContextGateBlocksHandoffOnlySubmit' core/internal/service/tasks/service_test.go
contains 'CONTEXT_HANDOFF_REQUIRED' core/internal/http/handlers/tasks.go core/internal/rpc/server.go
pass 'RV-016/RV-017/RV-018 task ownership and handoff-only guardrails covered'

# RV-019: generated local protocol explicitly rejects chat-history authority.
contains 'AI agents must not rely on chat history' cli/internal/cli/state.go
pass 'RV-019 no chat-history authority rule present in local protocol template'

# Contract consistency gates that can be checked statically.
contains_fixed '/api/projects/:projectId/tasks/:taskId/events' docs/API/tasks.md
contains_fixed '/api/projects/:projectId/tasks/:taskId/start' docs/API/tasks.md
contains_fixed '/api/projects/:projectId/tasks/:taskId/submit' docs/API/tasks.md
contains_fixed '/api/projects/:projectId/room/messages/:messageId/pin' docs/API/room.md
contains_fixed '/api/projects/:projectId/room/mentions/unread' docs/API/room.md
contains_fixed '/api/projects/:projectId/context/report' docs/API/context.md
contains_fixed '/api/projects/:projectId/bootstrap' docs/API/context.md
contains '禁止使用.*openapi.yaml' docs/API/README.md docs/后端/decisions.md
contains '不再维护.*mock' api/README.md docs/后端/README.md
contains 'agent-room-envelope' api/websocket/agent-room-envelope.schema.json api/websocket/agent-room-envelope.ts
contains 'CONTEXT_HANDOFF_REQUIRED' docs/API/health-errors.md core/internal/http/codes/codes.go web/src/api/errors.ts
contains 'version' cli/internal/cli/app.go docs/ai_agent_project_orchestrator_requirements_v2.md
pass 'contract smoke checks passed'

(
  cd core
  test -z "$(gofmt -l .)"
  go test ./... >/tmp/aitask-review-backend-tests.log
)
pass 'backend tests and gofmt gate passed'

(
  cd web
  pnpm lint >/tmp/aitask-review-frontend-lint.log
  pnpm test >/tmp/aitask-review-frontend-tests.log
)
pass 'frontend lint and unit tests passed'

if [[ "$RUN_RUNTIME" == "1" ]]; then
  bash scripts/review-runtime.sh >/tmp/aitask-review-runtime.log
  pass 'runtime review harness passed'
fi

if [[ "$RUN_PERF" == "1" ]]; then
  bash scripts/review-perf.sh >/tmp/aitask-review-perf.log
  pass 'performance review harness passed'
fi

if [[ "$RUN_DIFF" == "1" ]]; then
  bash scripts/review-diff.sh >/tmp/aitask-review-diff.log
  pass 'diff review harness executed'
fi

printf '[review-gate] DONE\n'
