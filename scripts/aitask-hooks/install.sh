#!/usr/bin/env bash
# Installs aitask SessionStart + live-monitor hooks for Claude Code, Codex CLI,
# and Gemini CLI.
#
# DEPRECATED: global hook install is no longer the recommended path. The
# AITask CLI now writes project-level AGENTS.md / CLAUDE.md / GEMINI.md as
# part of `aitask init`, which only takes effect inside initialized
# projects (no pollution of unrelated directories). The watch daemon is
# lazy-started by the CLI on first invocation inside an initialized
# project, so SessionStart hooks are no longer required.
#
# This installer is retained for two reasons:
#   1. To let existing users uninstall the legacy global hooks via
#      `install.sh --uninstall`.
#   2. To allow opt-in reinstallation via `install.sh --force ...` when a
#      user has a specific reason to keep the legacy flow.
#
# Usage:
#   scripts/aitask-hooks/install.sh --uninstall    # remove legacy hooks (recommended)
#   scripts/aitask-hooks/install.sh --force        # install legacy hooks anyway
#   scripts/aitask-hooks/install.sh --force claude # install Claude legacy hook only
#   scripts/aitask-hooks/install.sh --force codex
#   scripts/aitask-hooks/install.sh --force gemini
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATES="$DIR/templates"

print_deprecation_banner() {
  cat >&2 <<'EOF'
[aitask-hooks] DEPRECATED: global hook install is no longer recommended.
The new project-level flow writes AGENTS.md / CLAUDE.md / GEMINI.md via:
  aitask init --project <project_id>
Run this installer with --uninstall to remove the legacy global hooks, or
--force to proceed with global install anyway.
EOF
}

require_jq() {
  command -v jq >/dev/null 2>&1 || { echo "[aitask-hooks] jq is required for install" >&2; exit 1; }
}

link_script() {
  local src="$1" dest="$2"
  mkdir -p "$(dirname "$dest")"
  chmod +x "$src"
  ln -sfn "$src" "$dest"
  echo "[aitask-hooks] linked $dest -> $src"
}

merge_json() {
  # $1 target file, $2 patch file. Deep-merges patch into target (creates if missing).
  local target="$1" patch="$2"
  if [ -f "$target" ]; then
    local tmp
    tmp=$(mktemp)
    jq -s '.[0] * .[1]' "$target" "$patch" >"$tmp"
    mv "$tmp" "$target"
  else
    mkdir -p "$(dirname "$target")"
    cp "$patch" "$target"
  fi
  echo "[aitask-hooks] merged hooks block into $target"
}

install_claude() {
  link_script "$DIR/claude-session-start.sh" "$HOME/.claude/hooks/aitask-session-start.sh"
  merge_json "$HOME/.claude/settings.json" "$TEMPLATES/claude.settings.json"
  echo "[aitask-hooks] Claude Code legacy install complete."
}

install_codex() {
  link_script "$DIR/codex-session-start.sh" "$HOME/.codex/hooks/aitask-session-start.sh"
  link_script "$DIR/codex-prompt-submit.sh" "$HOME/.codex/hooks/aitask-prompt-submit.sh"
  merge_json "$HOME/.codex/hooks.json" "$TEMPLATES/codex.hooks.json"
  echo "[aitask-hooks] Codex legacy install complete. Note: Codex interactive TUI may skip SessionStart hooks (openai/codex#17532)."
}

install_gemini() {
  link_script "$DIR/gemini-session-start.sh" "$HOME/.gemini/hooks/aitask-session-start.sh"
  link_script "$DIR/gemini-before-agent.sh"  "$HOME/.gemini/hooks/aitask-before-agent.sh"
  merge_json "$HOME/.gemini/settings.json" "$TEMPLATES/gemini.settings.json"
  echo "[aitask-hooks] Gemini legacy install complete."
}

uninstall_all() {
  rm -f "$HOME/.claude/hooks/aitask-session-start.sh" \
        "$HOME/.codex/hooks/aitask-session-start.sh" \
        "$HOME/.codex/hooks/aitask-prompt-submit.sh" \
        "$HOME/.gemini/hooks/aitask-session-start.sh" \
        "$HOME/.gemini/hooks/aitask-before-agent.sh"
  echo "[aitask-hooks] Removed legacy hook script symlinks."
  echo "[aitask-hooks] Edit ~/.claude/settings.json, ~/.codex/hooks.json, and ~/.gemini/settings.json to drop the corresponding hook entries if no longer needed."
  echo "[aitask-hooks] For per-project setup going forward, run: aitask init --project <project_id>"
}

case "${1:-}" in
  --uninstall)
    uninstall_all
    exit 0
    ;;
  --force)
    require_jq
    target="${2:-all}"
    case "$target" in
      claude)   install_claude ;;
      codex)    install_codex ;;
      gemini)   install_gemini ;;
      all|"")   install_claude; install_codex; install_gemini ;;
      *) echo "usage: $0 --force [claude|codex|gemini|all]" >&2; exit 2 ;;
    esac
    exit 0
    ;;
  "")
    print_deprecation_banner
    exit 1
    ;;
  *)
    echo "usage: $0 [--uninstall | --force [claude|codex|gemini|all]]" >&2
    exit 2
    ;;
esac
