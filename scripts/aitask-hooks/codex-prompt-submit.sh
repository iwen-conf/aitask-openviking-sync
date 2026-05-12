#!/usr/bin/env bash
# Codex UserPromptSubmit hook: live-monitor parity. On every user turn, injects
# any aitask events (mentions / task delegations) that landed since the last hook
# invocation. Stays silent when nothing new arrived to keep prompts cheap.
# Output: {"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"..."}}
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

new="$(aitask_new_events_since_last codex)"
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
aitask_emit_hook_json "UserPromptSubmit" "$ctx"
