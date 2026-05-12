#!/usr/bin/env bash
# Gemini SessionStart hook: auto-launches aitask-watch daemon and seeds the
# session with the latest actionable mentions/task delegations.
# Output: {"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"..."}}
set -u

__src="${BASH_SOURCE[0]}"
while [ -L "$__src" ]; do
  __dir="$(cd -P "$(dirname "$__src")" && pwd)"
  __src="$(readlink "$__src")"
  [[ $__src != /* ]] && __src="$__dir/$__src"
done
DIR="$(cd -P "$(dirname "$__src")" && pwd)"
# shellcheck source=lib.sh
. "$DIR/lib.sh"

cat >/dev/null

lines=()
lines+=("$(aitask_ensure_watch_daemon)")

recent="$(aitask_recent_events 10)"
if [ -n "$recent" ]; then
  n=$(printf '%s\n' "$recent" | wc -l | tr -d ' ')
  lines+=("")
  lines+=("Last $n actionable aitask events (mentions + task delegations):")
  while IFS= read -r evt; do
    [ -n "$evt" ] && lines+=("  $evt")
  done <<< "$recent"
else
  lines+=("No recent mentions or task delegations in $EVENTS_FILE.")
fi

aitask_ensure_state_dir
size=$(wc -c <"$EVENTS_FILE" 2>/dev/null | tr -d ' ')
[ -n "${size:-}" ] && printf '%s' "$size" >"$STATE_DIR/gemini-prompt-offset" 2>/dev/null || true

lines+=("")
lines+=("Live in-session monitor: BeforeAgent hook will auto-inject any new mentions/delegations on every turn.")

ctx=$(printf '%s\n' "${lines[@]}")
aitask_emit_hook_json "SessionStart" "$ctx"
