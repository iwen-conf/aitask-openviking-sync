# aitask hooks: Legacy SessionStart Hooks (Deprecated)

> **DEPRECATED.** This directory ships the legacy global-install hooks for Claude Code, Codex CLI, and Gemini CLI. The current AITask flow uses **project-level context files** (`AGENTS.md` / `CLAUDE.md` / `GEMINI.md`) written by `aitask init`, plus **lazy daemon startup** from the `aitask` CLI itself. New users should not run `install.sh`.
>
> The scripts here are retained so existing installations can be cleanly uninstalled and so power users can opt back in via `install.sh --force` when they have a specific reason to.

## Why the change

The old install path wired SessionStart hooks into `~/.claude/settings.json`, `~/.codex/hooks.json`, and `~/.gemini/settings.json` — making them global across every project. Side effects:

- `aitask-watch` was launched in every shell session regardless of whether the working directory was an AITask project (this broke unrelated repos).
- Codex 0.120.0–0.128.0 has a confirmed bug ([openai/codex#17532](https://github.com/openai/codex/issues/17532)) where interactive TUI sessions skip SessionStart hooks entirely, so the global Codex hook was already half-broken in practice.

The replacement is project-scoped and works in interactive and non-interactive modes alike:

| Agent | Context file | How it loads |
| --- | --- | --- |
| Claude Code | `CLAUDE.md` (repo root) | Auto-injected into the system prompt |
| Codex CLI | `AGENTS.md` (repo root) | Read on every session start, both `codex` TUI and `codex exec` |
| Gemini CLI | `GEMINI.md` (repo root) | Hierarchical context loader |

`aitask init` writes a marker-delimited `<!-- BEGIN aitask:context --> ... <!-- END aitask:context -->` block into each file, preserving any user content outside the markers. Re-running `aitask init` updates the block in place; the rest of the file is untouched.

The `aitask-watch` daemon is now launched lazily by the CLI itself: any `aitask <subcommand>` invoked inside an initialized project (i.e. `.aitask/project.md` present) will start the tmux session on demand. Outside an initialized project, the daemon stays dormant.

## Recommended migration

```bash
# 1. Remove the legacy global hooks
scripts/aitask-hooks/install.sh --uninstall

# 2. In each project you actually use with aitask, run:
cd /path/to/project
aitask init --project <project_id>
```

To select a subset of agents during `init`:

```bash
aitask init --project <project_id> --agents codex,claude
```

## Re-enabling legacy hooks (advanced)

If you need the old SessionStart hooks for a specific reason (e.g. running aitask from an uninitialized directory and wanting daemon auto-launch), you can opt back in:

```bash
scripts/aitask-hooks/install.sh --force         # install all three
scripts/aitask-hooks/install.sh --force claude  # Claude only
scripts/aitask-hooks/install.sh --force codex   # Codex only (subject to openai/codex#17532)
scripts/aitask-hooks/install.sh --force gemini  # Gemini only
```

The bare command (with no flag) prints a deprecation banner and exits non-zero to avoid accidental installs.

## File layout

```
scripts/aitask-hooks/
├── lib.sh                         # shared helpers (kept for legacy --force install)
├── claude-session-start.sh        # Claude Code SessionStart hook (legacy)
├── codex-session-start.sh         # Codex SessionStart hook (legacy)
├── codex-prompt-submit.sh         # Codex UserPromptSubmit (legacy)
├── gemini-session-start.sh        # Gemini SessionStart hook (legacy)
├── gemini-before-agent.sh         # Gemini BeforeAgent hook (legacy)
├── install.sh                     # legacy install / uninstall driver
└── templates/                     # settings.json fragments used by --force
```

## Environment overrides (legacy)

These environment variables remain in effect for both the legacy hooks and the new lazy-daemon path inside the `aitask` CLI:

| Var | Default | Purpose |
| --- | --- | --- |
| `AITASK_EVENTS_FILE` | `~/.aitask/events.ndjson` | Source NDJSON event stream |
| `AITASK_HOOK_STATE_DIR` | `~/.aitask/hook-state` | Per-agent byte-offset state (legacy hooks only) |
| `AITASK_WATCH_TMUX` | `aitask-watch` | tmux session name for the daemon |
| `AITASK_WATCH_BIN` | `aitask-watch` | watch daemon binary name |
| `AITASK_WATCH_ARGS` | `--notify auto --stdout=false` | args passed to the daemon |
