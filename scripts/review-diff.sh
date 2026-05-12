#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/scripts/lib/review-common.sh"

review_require jq
review_require python3
review_require diff

ARTIFACT_DIR="${ARTIFACT_DIR:-$DIFF_ROOT}"
TMP_DIFF_DIR="$ARTIFACT_DIR/tmp"
SUMMARY_JSON="$ARTIFACT_DIR/summary.json"
SUMMARY_MD="$ARTIFACT_DIR/summary.md"

mkdir -p "$ARTIFACT_DIR" "$TMP_DIFF_DIR"

normalize_openviking_doc() {
  python3 - "$ROOT_DIR/docs/ai_agent_project_orchestrator_requirements_v2.md" >"$TMP_DIFF_DIR/doc-openviking.txt" <<'PY'
import pathlib, re, sys
text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
m = re.search(r"### 5\.2 OpenViking 目录结构\s+```text\n(.*?)\n```", text, re.S)
if not m:
    raise SystemExit("missing doc section 5.2")
lines = []
for raw in m.group(1).splitlines():
    line = raw.rstrip()
    if not line.strip():
        continue
    line = re.sub(r"^viking://aitask/projects/\{project_id\}/", "", line)
    line = re.sub(r"^[\s│├└─]+", "", line)
    line = line.replace("{session_id}", "<session_id>")
    line = line.strip()
    if line:
        lines.append(line)
print("\n".join(lines))
PY
}

normalize_openviking_actual() {
  local source="${1:?openviking tree source required}"
  python3 - "$source" >"$TMP_DIFF_DIR/actual-openviking.txt" <<'PY'
import pathlib, sys
root = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8").splitlines()
items = []
for line in root:
    value = line.strip()
    if not value:
        continue
    if "/projects/" not in value:
        continue
    rel = value.split("/projects/", 1)[1]
    parts = rel.split("/", 1)
    if len(parts) == 1:
        continue
    rel = parts[1]
    rel = rel.replace(".md", "")
    items.append(rel)
print("\n".join(items))
PY
}

normalize_aitask_doc() {
  python3 - "$ROOT_DIR/docs/ai_agent_project_orchestrator_requirements_v2.md" >"$TMP_DIFF_DIR/doc-aitask.txt" <<'PY'
import pathlib, re, sys
text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
m = re.search(r"### 6\.1 本地目录结构\s+.*?```text\n(.*?)\n```", text, re.S)
if not m:
    raise SystemExit("missing doc section 6.1")
lines = []
for raw in m.group(1).splitlines():
    line = raw.rstrip()
    if not line.strip():
        continue
    line = re.sub(r"^project-root/", "", line)
    line = re.sub(r"^[\s│├└─]+", "", line)
    line = line.replace(".md", "")
    line = line.strip()
    if line:
      lines.append(line)
print("\n".join(lines))
PY
}

normalize_aitask_actual() {
  local workspace="${1:?workspace required}"
  (
    cd "$workspace"
    find .aitask -print | sort | sed 's#^\./##; s#\.md$##'
  ) >"$TMP_DIFF_DIR/actual-aitask.txt"
}

normalize_proto_doc() {
  python3 - "$ROOT_DIR/docs/ai_agent_project_orchestrator_requirements_v2.md" >"$TMP_DIFF_DIR/doc-proto.txt" <<'PY'
import pathlib, re, sys
text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
sections = []
for anchor in ("8.2 `aitask/v1/common.proto`", "8.3 `aitask/v1/context.proto`", "8.4 `aitask/v1/task.proto`", "8.5 `aitask/v1/bootstrap.proto`"):
    m = re.search(rf"### {re.escape(anchor)}\s+```proto\n(.*?)\n```", text, re.S)
    if not m:
        raise SystemExit(f"missing proto section {anchor}")
    body = "\n".join(line.rstrip() for line in m.group(1).splitlines())
    sections.append(f"### {anchor}\n{body}")
print("\n\n".join(sections))
PY
}

normalize_proto_actual() {
  {
    printf '### 8.2 `aitask/v1/common.proto`\n'
    sed 's/[[:space:]]\+$//' "$ROOT_DIR/api/protobuf/aitask/v1/common.proto"
    printf '\n### 8.3 `aitask/v1/context.proto`\n'
    sed 's/[[:space:]]\+$//' "$ROOT_DIR/api/protobuf/aitask/v1/context.proto"
    printf '\n### 8.4 `aitask/v1/task.proto`\n'
    sed 's/[[:space:]]\+$//' "$ROOT_DIR/api/protobuf/aitask/v1/task.proto"
    printf '\n### 8.5 `aitask/v1/bootstrap.proto`\n'
    sed 's/[[:space:]]\+$//' "$ROOT_DIR/api/protobuf/aitask/v1/bootstrap.proto"
  } >"$TMP_DIFF_DIR/actual-proto.txt"
}

normalize_db_doc() {
  python3 - "$ROOT_DIR/docs/ai_agent_project_orchestrator_requirements_v2.md" >"$TMP_DIFF_DIR/doc-db.txt" <<'PY'
import pathlib, re, sys
text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
tables = []
for idx in range(1, 21):
    anchor = rf"### 12\.{idx}\s+([^\n]+)\s+```sql\n(.*?)\n```"
    m = re.search(anchor, text, re.S)
    if not m:
        raise SystemExit(f"missing db section 12.{idx}")
    name = m.group(1).strip()
    body = "\n".join(line.rstrip() for line in m.group(2).splitlines())
    tables.append(f"### 12.{idx} {name}\n{body}")
print("\n\n".join(tables))
PY
}

normalize_db_actual() {
  local INIT_SQL="$ROOT_DIR/migrations/postgres/000001_init.up.sql"
  {
    printf '### 12.1 projects\n'
    sed -n '/^CREATE TABLE projects /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.2 project_sessions\n'
    sed -n '/^CREATE TABLE project_sessions /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.3 agents\n'
    sed -n '/^CREATE TABLE agents /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.4 agent_project_bindings\n'
    sed -n '/^CREATE TABLE agent_project_bindings /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.5 agent_tokens\n'
    sed -n '/^CREATE TABLE agent_tokens /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.6 agent_skills\n'
    sed -n '/^CREATE TABLE agent_skills /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.7 agent_models\n'
    sed -n '/^CREATE TABLE agent_models /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.8 agent_runs\n'
    sed -n '/^CREATE TABLE agent_runs /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.9 tasks\n'
    sed -n '/^CREATE TABLE tasks /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.10 task_required_skills\n'
    sed -n '/^CREATE TABLE task_required_skills /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.11 task_dependencies\n'
    sed -n '/^CREATE TABLE task_dependencies /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.12 task_delegations\n'
    sed -n '/^CREATE TABLE task_delegations /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.13 task_events\n'
    sed -n '/^CREATE TABLE task_events /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.14 artifacts\n'
    sed -n '/^CREATE TABLE artifacts /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.15 project_rooms\n'
    sed -n '/^CREATE TABLE project_rooms /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.16 project_room_members\n'
    sed -n '/^CREATE TABLE project_room_members /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.17 project_room_messages\n'
    sed -n '/^CREATE TABLE project_room_messages /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.18 project_room_mentions\n'
    sed -n '/^CREATE TABLE project_room_mentions /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.19 context_handoffs\n'
    sed -n '/^CREATE TABLE context_handoffs /,/^);$/p' "$INIT_SQL"
    printf '\n### 12.20 agent_run_context_usage\n'
    sed -n '/^CREATE TABLE agent_run_context_usage /,/^);$/p' "$INIT_SQL"
  } >"$TMP_DIFF_DIR/actual-db.txt"
}

normalize_scope_doc() {
  python3 - "$ROOT_DIR/docs/ai_agent_project_orchestrator_requirements_v2.md" >"$TMP_DIFF_DIR/doc-scopes.txt" <<'PY'
import pathlib, re, sys
text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
blocks = []
for idx, title in (("19.4", "Claude"), ("19.5", "Codex"), ("19.6", "Gemini")):
    m = re.search(rf"### {re.escape(idx)} .*?\n\n```text\n(.*?)\n```", text, re.S)
    if not m:
        raise SystemExit(f"missing scope section {idx}")
    scopes = [line.strip() for line in m.group(1).splitlines() if line.strip()]
    blocks.append(f"[{title}]\n" + "\n".join(scopes))
print("\n\n".join(blocks))
PY
}

normalize_scope_actual() {
  python3 - "$ROOT_DIR/core/internal/service/agents/defaults.go" >"$TMP_DIFF_DIR/actual-scopes.txt" <<'PY'
import pathlib, re, sys
text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
mapping = [("claude-code", "Claude"), ("codex", "Codex"), ("gemini", "Gemini")]
blocks = []
for key, title in mapping:
    m = re.search(rf'"{re.escape(key)}": \{{.*?Scopes: \[\](.*?)\],\n\s+Skills:', text, re.S)
    if not m:
        raise SystemExit(f"missing scopes for {key}")
    scopes = re.findall(r'"([^"]+)"', m.group(1))
    blocks.append(f"[{title}]\n" + "\n".join(scopes))
print("\n\n".join(blocks))
PY
}

normalize_ws_doc() {
  python3 - "$ROOT_DIR/docs/ai_agent_project_orchestrator_requirements_v2.md" >"$TMP_DIFF_DIR/doc-ws-events.txt" <<'PY'
import pathlib, re, sys
text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
m = re.search(r"### 10\.4 事件类型\s+```text\n(.*?)\n```", text, re.S)
if not m:
    raise SystemExit("missing ws event section 10.4")
events = [line.strip() for line in m.group(1).splitlines() if line.strip()]
print("\n".join(events))
PY
}

normalize_ws_actual() {
  python3 - "$ROOT_DIR/api/websocket/agent-room-envelope.ts" "$ROOT_DIR/core/internal/service/room/service.go" >"$TMP_DIFF_DIR/actual-ws-events.txt" <<'PY'
import pathlib, re, sys
ts = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
svc = pathlib.Path(sys.argv[2]).read_text(encoding="utf-8")
events = set(re.findall(r"\| '([^']+)'", ts))
events.update(re.findall(r'EventType: "([^"]+)"', svc))
events.update(re.findall(r'PublishPresence\(.*?"([^"]+)"', svc))
print("\n".join(sorted(events)))
PY
}

run_diff() {
  local label="$1"
  local expected="$2"
  local actual="$3"
  local diff_file="$ARTIFACT_DIR/${label}.diff"
  if diff -u "$expected" "$actual" >"$diff_file"; then
    printf 'PASS\n' >"$ARTIFACT_DIR/${label}.status"
  else
    printf 'FAIL\n' >"$ARTIFACT_DIR/${label}.status"
  fi
}

review_diff_prepare_runtime_artifacts() {
  if [[ ! -f "$RUNTIME_ROOT/openviking-tree.txt" || ! -f "$CLAUDE_WORKDIR_FILE" ]]; then
    AITASK_WORKER_ENABLED="${AITASK_WORKER_ENABLED:-true}" bash "$ROOT_DIR/scripts/review-runtime.sh"
  fi
}

review_diff_prepare_runtime_artifacts

normalize_proto_doc
normalize_proto_actual
run_diff "rv-051-proto" "$TMP_DIFF_DIR/doc-proto.txt" "$TMP_DIFF_DIR/actual-proto.txt"

normalize_db_doc
normalize_db_actual
run_diff "rv-054-db" "$TMP_DIFF_DIR/doc-db.txt" "$TMP_DIFF_DIR/actual-db.txt"

normalize_scope_doc
normalize_scope_actual
run_diff "rv-056-scopes" "$TMP_DIFF_DIR/doc-scopes.txt" "$TMP_DIFF_DIR/actual-scopes.txt"

normalize_openviking_doc
normalize_openviking_actual "$RUNTIME_ROOT/openviking-tree.txt"
run_diff "rv-057-openviking" "$TMP_DIFF_DIR/doc-openviking.txt" "$TMP_DIFF_DIR/actual-openviking.txt"

normalize_aitask_doc
normalize_aitask_actual "$(cat "$CLAUDE_WORKDIR_FILE")"
run_diff "rv-058-aitask" "$TMP_DIFF_DIR/doc-aitask.txt" "$TMP_DIFF_DIR/actual-aitask.txt"

normalize_ws_doc
normalize_ws_actual
run_diff "rv-059-ws-events" "$TMP_DIFF_DIR/doc-ws-events.txt" "$TMP_DIFF_DIR/actual-ws-events.txt"

jq -n \
  --arg rv051 "$(cat "$ARTIFACT_DIR/rv-051-proto.status")" \
  --arg rv054 "$(cat "$ARTIFACT_DIR/rv-054-db.status")" \
  --arg rv056 "$(cat "$ARTIFACT_DIR/rv-056-scopes.status")" \
  --arg rv057 "$(cat "$ARTIFACT_DIR/rv-057-openviking.status")" \
  --arg rv058 "$(cat "$ARTIFACT_DIR/rv-058-aitask.status")" \
  --arg rv059 "$(cat "$ARTIFACT_DIR/rv-059-ws-events.status")" \
  '{
    diff: {
      rv051: $rv051,
      rv054: $rv054,
      rv056: $rv056,
      rv057: $rv057,
      rv058: $rv058,
      rv059: $rv059
    }
  }' >"$SUMMARY_JSON"

cat >"$SUMMARY_MD" <<EOF
# Diff Review Summary

- RV-051 proto-doc diff: \`$(cat "$ARTIFACT_DIR/rv-051-proto.status")\`
- RV-054 schema-doc diff: \`$(cat "$ARTIFACT_DIR/rv-054-db.status")\`
- RV-056 default scopes diff: \`$(cat "$ARTIFACT_DIR/rv-056-scopes.status")\`
- RV-057 openviking tree diff: \`$(cat "$ARTIFACT_DIR/rv-057-openviking.status")\`
- RV-058 .aitask layout diff: \`$(cat "$ARTIFACT_DIR/rv-058-aitask.status")\`
- RV-059 websocket events diff: \`$(cat "$ARTIFACT_DIR/rv-059-ws-events.status")\`
EOF

review_log "diff evidence written to ${ARTIFACT_DIR}"
