#!/usr/bin/env bash
# Claude Code SessionStart hook: auto-launches aitask-watch daemon and seeds
# the session with the latest actionable mentions/task delegations.
# Claude has no per-turn equivalent of Codex UserPromptSubmit / Gemini
# BeforeAgent — live push is delivered via the Monitor tool tailing the NDJSON
# stream, so we surface that hint at the end.
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

lines+=("")
lines+=("For live in-session push, launch a Monitor on: tail -F -n 0 ~/.aitask/events.ndjson")

ctx=$(printf '%s\n' "${lines[@]}")
aitask_emit_hook_json "SessionStart" "$ctx"
