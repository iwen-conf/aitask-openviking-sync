#!/usr/bin/env bash
# shared helpers for aitask hook scripts (codex / gemini / claude parity)

EVENTS_FILE="${AITASK_EVENTS_FILE:-$HOME/.aitask/events.ndjson}"
STATE_DIR="${AITASK_HOOK_STATE_DIR:-$HOME/.aitask/hook-state}"
WATCH_SESSION="${AITASK_WATCH_TMUX:-aitask-watch}"
WATCH_BIN="${AITASK_WATCH_BIN:-aitask-watch}"
WATCH_ARGS="${AITASK_WATCH_ARGS:---notify auto --stdout=false}"

aitask_log() { printf '[aitask-hook] %s\n' "$*" >&2; }

aitask_ensure_state_dir() {
  [ -d "$STATE_DIR" ] || mkdir -p "$STATE_DIR" 2>/dev/null || true
}

# Returns 0 if tmux watch session is alive, 1 otherwise.
aitask_watch_running() {
  command -v tmux >/dev/null 2>&1 || return 1
  tmux has-session -t "$WATCH_SESSION" 2>/dev/null
}

# Auto-launches the watch daemon inside a tmux session if missing.
# Echoes a one-line status string for inclusion in additionalContext.
aitask_ensure_watch_daemon() {
  if aitask_watch_running; then
    printf 'aitask-watch daemon is running (tmux session: %s).' "$WATCH_SESSION"
    return 0
  fi
  if ! command -v tmux >/dev/null 2>&1; then
    printf 'aitask-watch daemon NOT running and tmux not found on PATH. Install tmux or start daemon manually: %s %s' "$WATCH_BIN" "$WATCH_ARGS"
    return 1
  fi
  if ! command -v "$WATCH_BIN" >/dev/null 2>&1; then
    printf 'aitask-watch daemon NOT running and binary "%s" not found on PATH. Install the AITask CLI suite first: brew install iwen-conf/tap/aitask' "$WATCH_BIN"
    return 1
  fi
  if tmux new-session -d -s "$WATCH_SESSION" "$WATCH_BIN $WATCH_ARGS" 2>/dev/null; then
    printf 'aitask-watch daemon auto-started (tmux session: %s, cmd: %s %s).' "$WATCH_SESSION" "$WATCH_BIN" "$WATCH_ARGS"
    return 0
  fi
  printf 'aitask-watch daemon FAILED to auto-start. Run manually: tmux new -ds %s "%s %s"' "$WATCH_SESSION" "$WATCH_BIN" "$WATCH_ARGS"
  return 1
}

# Tail recent actionable events (mentions + task delegations).
# Args: max_lines (default 10)
aitask_recent_events() {
  local n="${1:-10}"
  [ -f "$EVENTS_FILE" ] || return 0
  command -v jq >/dev/null 2>&1 || { tail -n "$n" "$EVENTS_FILE" 2>/dev/null; return; }
  tail -n 200 "$EVENTS_FILE" 2>/dev/null \
    | jq -c 'select(.kind == "mention" or .kind == "task_delegated")' 2>/dev/null \
    | tail -n "$n"
}

# Stateful tail: emit only NEW actionable events since last call, then advance offset.
# Args: state_key (e.g. "codex" or "gemini")
aitask_new_events_since_last() {
  local key="${1:-default}"
  [ -f "$EVENTS_FILE" ] || return 0
  aitask_ensure_state_dir
  local offset_file="$STATE_DIR/${key}-prompt-offset"
  local size last_offset new_bytes
  size=$(wc -c <"$EVENTS_FILE" 2>/dev/null | tr -d ' ')
  size=${size:-0}
  if [ -f "$offset_file" ]; then
    last_offset=$(cat "$offset_file" 2>/dev/null | tr -d ' \n')
    last_offset=${last_offset:-0}
  else
    # First run: arm offset at current EOF so we don't dump history.
    printf '%s' "$size" >"$offset_file" 2>/dev/null
    return 0
  fi
  if [ "$size" -lt "$last_offset" ]; then
    # File rotated/truncated: re-arm.
    printf '%s' "$size" >"$offset_file" 2>/dev/null
    return 0
  fi
  if [ "$size" -eq "$last_offset" ]; then
    return 0
  fi
  new_bytes=$((size - last_offset))
  if command -v jq >/dev/null 2>&1; then
    tail -c "$new_bytes" "$EVENTS_FILE" 2>/dev/null \
      | jq -c 'select(.kind == "mention" or .kind == "task_delegated")' 2>/dev/null
  else
    tail -c "$new_bytes" "$EVENTS_FILE" 2>/dev/null
  fi
  printf '%s' "$size" >"$offset_file" 2>/dev/null
}

# Emit a hook JSON object. Args: hookEventName, additionalContext text.
aitask_emit_hook_json() {
  local event="$1"
  local ctx="$2"
  if command -v jq >/dev/null 2>&1; then
    jq -nc --arg e "$event" --arg c "$ctx" \
      '{hookSpecificOutput: {hookEventName: $e, additionalContext: $c}}'
  else
    # Minimal escape: backslash, double-quote, newline.
    local esc
    esc=$(printf '%s' "$ctx" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' | awk 'BEGIN{ORS=""}{if(NR>1)printf "\\n"; print}')
    printf '{"hookSpecificOutput":{"hookEventName":"%s","additionalContext":"%s"}}' "$event" "$esc"
  fi
}
