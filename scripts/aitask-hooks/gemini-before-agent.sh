#!/usr/bin/env bash
# Gemini BeforeAgent hook: live-monitor parity. Fires after each user prompt
# but before the agent plans, injecting any new aitask events seen since the
# previous turn. Stays silent when nothing new arrived.
# Output: {"hookSpecificOutput":{"hookEventName":"BeforeAgent","additionalContext":"..."}}
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

new="$(aitask_new_events_since_last gemini)"
if [ -z "$new" ]; then
  exit 0
fi

n=$(printf '%s\n' "$new" | wc -l | tr -d ' ')
lines=()
lines+=("[aitask live monitor] $n new actionable event(s) since last turn:")
while IFS= read -r evt; do
  [ -n "$evt" ] && lines+=("  $evt")
done <<< "$new"
lines+=("")
lines+=("Triage the mentions above before continuing this turn if any target this agent.")

ctx=$(printf '%s\n' "${lines[@]}")
aitask_emit_hook_json "BeforeAgent" "$ctx"
