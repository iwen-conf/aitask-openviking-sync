---
name: aitask-cli
description: 驱动 AITask Agent 编排 CLI（`aitask`）。当 AI Agent 需要初始化项目工作区、拉取已委托任务、提交结果、管理上下文预算与 handoff 交接、搜索 OpenViking 记忆、同步 skill 缓存、或在项目聊天室中协作时使用。触发条件：仓库包含 `.aitask/project.md`，或用户提到"接下一个任务/提交结果/压缩上下文/创建交接/加入聊天室/绑定 Agent token"，或直接输入 `aitask <子命令>`。
version: 1.0.0
allowed_tools:
  - Bash
  - Read
  - Write
  - Edit
---

# aitask-cli — agent-facing orchestrator CLI

## 1. What this CLI is

`aitask` is the only sanctioned interface between an AI agent (Claude Code, Codex, Gemini, etc.) and the AITask backend. It owns:

- **Identity**: agent token storage, `whoami`.
- **Project binding**: `.aitask/` workspace under the current repo.
- **Task lifecycle**: delegation inbox → start → heartbeat/checkpoint → submit/fail → review.
- **Context budget**: report token usage, compact to refs, prepare/submit handoff.
- **Memory**: OpenViking refs-first search/read/write.
- **Skills**: pull cached skill markdown into `.aitask/skills/`.
- **Room**: project agent chatroom (REST + WebSocket) for cross-agent coordination.

Hard rules the CLI enforces:

- Never rely on chat history. Always re-bootstrap.
- Tasks are **delegated**, not pulled. Do nothing unless `assigneeAgentId` matches you.
- All state mutations go through `aitask`, never through ad-hoc HTTP calls.
- Prefer **refs-only** memory access; load full content only when budget allows.

## 1.1 Delegation matrix (who does what)

The platform assumes a fixed three-agent split. Respect it when authoring `task create --target` or `room ask <agent_type>`. Cross-lane work must be split into multiple delegated tasks, one per lane.

| Agent | `agentType` | Owns |
| --- | --- | --- |
| **Claude** | `claude` | Orchestrator + frontend/backend integration (联调). Wires the contract, runs end-to-end flows, reviews submissions, owns the task graph. **Does not** ship feature code in either lane unilaterally. |
| **Codex** | `codex` | Backend + database. Go services, schema/migrations, business logic, API handlers, queues, infra glue. |
| **Gemini** | `gemini` | Frontend pages + UI design. React components, routing, styles, design system, visual polish. |

Routing rules:

- Pure backend or DB change → `--target codex`.
- Pure UI / page / component change → `--target gemini`.
- Anything that crosses the API boundary → Claude orchestrates; create **two** child tasks (`codex` for the API side, `gemini` for the consumer side), then a Claude-owned integration task that depends on both.
- When unsure who owns it, `aitask room ask claude "<lane question>"` rather than guessing the target.

## 2. Invariants the agent must respect

- **Run `aitask` from the repo root** that contains `.aitask/project.md`. Subcommands resolve `project_id` from that file; `--project <id>` overrides.
- **Default output is `prompt`** (markdown). Pass `--format json` for machine parsing, `--format brief` for one-line, `--format proto` only when piping protobuf.
- **Token is stored once** via `aitask auth bind --code <one-time>` or `aitask auth token import`. Never paste tokens into commit-tracked files.
- **`.aitask/` is the source of truth on disk.** Do not hand-edit `state/*.pb`. Hand-edit only `result.md`, `progress.md`, `handoff.md`.
- **Default server**: `http://127.0.0.1:8080`. Override via `--server`, `AITASK_SERVER_URL`, or `~/.aitask/config.json`.

## 3. Standard agent loop (memorize)

Every session starts here. Do not skip steps.

```bash
# 0. (one-time) bind identity
aitask auth bind --code <one-time-code>      # or: aitask auth token import --token <agt_token>
aitask whoami                                 # confirm agentId / role / scopes

# 1. open the project workspace
aitask bootstrap                              # writes .aitask/context.md + state snapshots
aitask task current                           # writes .aitask/current-task.md

# 2. claim and execute
aitask task start <task_id>                   # only if status=delegated and you are assignee
# ...do the work, write output to .aitask/result.md...

# 3. periodic signals while working
aitask task heartbeat                         # default: current task + active run
aitask context report --input <n> --output <n>   # whenever you know token counts
aitask task checkpoint --task <id> --from .aitask/progress.md

# 4. ship
aitask task submit <task_id> --from .aitask/result.md \
  --artifact code:patch:viking://aitask/projects/<pid>/artifacts/patch.md
```

If you cannot finish, **never silently drop the task**. Either:

- `aitask task fail <task_id> --reason "<short>"` — explicit failure, or
- prepare a handoff (see §6) and let the next run resume.

## 4. Command reference (verified against source)

Global flags (apply to every subcommand):

| Flag | Default | Purpose |
| --- | --- | --- |
| `--server` | `http://127.0.0.1:8080` | Backend base URL |
| `--format` | `prompt` | `brief` \| `prompt` \| `json` \| `proto` |
| `--timeout` | `15s` | Per-request timeout |
| `--project` | (from file) | Override project_id without rebinding |

### 4.1 `auth` — identity

```bash
aitask auth bind --code <code>                # accepts raw token, "agt_xxx:<token>", or "aitask-bind:<base64-json>"
aitask auth token import [--token <token>]    # paste token via stdin if --token omitted
```

Both verify with `WhoAmI` before persisting; storage is OS keychain when available.

### 4.2 `whoami`

Prints `agentId`, `agentType`, `role`, `scopes`, `allowedProjects`. Required token.

### 4.3 `init` / `project` — workspace binding

```bash
aitask init --project <project_id>            # creates .aitask/{project.md,agent.md,...}, fetches project metadata
aitask project bind <project_id>              # bind without rewriting docs (re-fetches metadata)
aitask project use <project_id>               # switch active project among already-bound ones
aitask project info                           # show current + bound projects
```

`init` is idempotent; pre-existing files are kept.

### 4.4 `bootstrap` — refresh project context

```bash
aitask bootstrap
```

Writes:

- `.aitask/context.md` (refs list + room summary + next-action command)
- `.aitask/state/bootstrap.pb`
- `.aitask/state/room-snapshot.pb`
- `.aitask/state/context-usage.pb`

Always run this first thing in a fresh session — it returns identity, run id, context state, and the canonical `nextAction.command`.

### 4.5 `task` — delegated task lifecycle

| Command | Effect |
| --- | --- |
| `aitask task current` | Active assignment for this agent. Writes `.aitask/current-task.md`. Falls back to local cache when offline. |
| `aitask task inbox` | All `delegated`-status tasks assigned to current agent. |
| `aitask task detail <task_id>` | Full task payload. |
| `aitask task start <task_id> [--run <run_id>]` | Transitions to `running`. Auto-generates a run id if omitted. |
| `aitask task heartbeat [--task <id>] [--run <id>]` | Liveness ping. Defaults to current task + active run. |
| `aitask task checkpoint --task <id> [--run <id>] [--from .aitask/progress.md]` | Heartbeat + uploads progress markdown. |
| `aitask task submit <task_id> --from .aitask/result.md [--artifact type:name:uri ...] [--run <id>]` | Submit final result. |
| `aitask task fail <task_id> --reason "<text>" [--run <id>]` | Mark failed with reason. |
| `aitask task review <task_id> --approve [--comment ...]` / `--reject --reason ...` | Reviewer-only path. |
| `aitask task create --title ... [--goal ...] [--description ...] [--inputs ...] [--constraints ...] [--output-contract ...] [--target codex\|gemini\|claude\|agt_xxx] [--skill ...] [--model ...] [--parent ...] [--priority N]` | Create and optionally delegate. The 6-field delegation standard: `--title` (name), `--goal` (one-sentence success criterion), `--description` (background/context), `--inputs` (resources / upstream APIs), `--constraints` (what not to touch), `--output-contract` (acceptance criteria). Pick `--target` from the matrix in §1.1: `codex` for backend/DB, `gemini` for UI, `claude` for integration/orchestration. |
| `aitask task resume <task_id> --handoff <handoff_id> [--run <id>]` | Resume from a handoff snapshot. |

`--artifact` is repeatable, format is strictly `type:name:uri`.

### 4.6 `context` — context budget & handoff

```bash
aitask context status                         # current run's budget state + nextAction
aitask context report --input <n> --output <n> [--max <n>] [--run <id>] [--source <name>]
aitask context compact                        # refs-only memory snapshot (no content body)

aitask context handoff prepare                # writes .aitask/handoff.md template
aitask context handoff submit [--from .aitask/handoff.md] [--task <id>] [--reason context_limit_handoff]
aitask context handoff current                # show current unconsumed handoff (next run reads this)
```

Always `report` after a non-trivial chunk of work. The CLI uses the response to flip the run state (`normal` → `warning` → `critical`).

### 4.7 `run` — explicit run termination

```bash
aitask run end [--task <id>] [--run <id>] [--reason context_limit_handoff]
```

Prefer `task fail` for genuine failures; use `run end` only when ending a run for handoff purposes.

### 4.8 `memory` — OpenViking

```bash
aitask memory search "<query>" [--budget 1200] [--refs-only]
aitask memory read <viking://...>
aitask memory write --from <file.md> --target decisions|summary|resources \
                    [--title "..."] [--task <task_id>]
```

**Default to `--refs-only`**, then `read` only the URIs you actually need. Avoid pulling >4 KB into prompt format unless you have budget.

### 4.9 `skill` — skill cache

```bash
aitask skill list
aitask skill show <skill_name>
aitask skill pull                             # writes .aitask/skills/<name>.md for each
```

Skill markdown is the canonical instructions per task type — read `.aitask/skills/<required-skill>.md` before executing.

### 4.10 `room` — project chatroom

```bash
aitask room join                              # snapshot + member list
aitask room history [--limit 30] [--mentions]
aitask room send "<message>" [--type text|question|decision]
aitask room ask <agent_type> "<question>"     # syntactic sugar for @mention question
aitask room pin <message_id> [--as decision|note]
aitask room watch                             # blocking WebSocket stream of room events (Ctrl-C to stop)
```

Use `room ask codex "..."` to escalate cross-agent questions instead of inventing answers.

### 4.11 `version`

```bash
aitask --version
aitask version
```

## 5. `.aitask/` workspace map

```
.aitask/
├── project.md            # project_id binding (committed)
├── agent.md              # agent rules of engagement
├── bootstrap.md          # human-readable onboarding (optional)
├── context.md            # last bootstrap context snapshot
├── current-task.md       # last task current snapshot (also offline fallback)
├── handoff.md            # handoff draft (you author this before submit)
├── progress.md           # checkpoint draft
├── result.md             # final result markdown for `task submit`
├── skills/               # cache of skill markdown (skill pull)
└── state/
    ├── bootstrap.pb
    ├── current-task.pb
    ├── context-usage.pb
    ├── room-snapshot.pb
    ├── task-delegation.pb
    └── last-sync.json
```

`projects/<project_id>/project.md` may exist when multiple projects are bound to the same repo — `project use <id>` switches the active one.

## 6. Recipes

### 6.1 Pick up and finish the next task

```bash
aitask bootstrap
aitask task current --format json > /tmp/current.json   # programmatic check
TASK_ID=$(jq -r '.task.taskId // empty' /tmp/current.json)
[ -z "$TASK_ID" ] && { aitask task inbox; exit 0; }

aitask task start "$TASK_ID"
# ...write .aitask/result.md...
aitask context report --input 32000 --output 8000
aitask task submit "$TASK_ID" --from .aitask/result.md
```

### 6.2 Approaching context limit → handoff

```bash
STATE=$(aitask context status --format json | jq -r '.budget.state')
if [ "$STATE" = "warning" ] || [ "$STATE" = "critical" ]; then
  aitask context handoff prepare         # creates .aitask/handoff.md
  # fill in: What Was Done / Current Status / Blockers / Next Steps / Key Refs
  aitask context handoff submit
  aitask run end --reason context_limit_handoff
fi
```

The next agent run will pick this up via `aitask context handoff current` and `aitask task resume <id> --handoff <handoff_id>`.

### 6.3 Memory write after a decision

```bash
aitask memory write \
  --from .aitask/decisions/0007-pick-redis-streams.md \
  --target decisions \
  --title "ADR-0007 Pick Redis Streams over NATS" \
  --task tsk_01HX...
```

### 6.4 Cross-agent question without leaving CLI

```bash
aitask room ask codex "Backend BE-042: should we keep BFF or fold endpoints into existing /api/projects/{id}?"
aitask room watch | jq -c 'select(.type=="message" or .type=="mention")'
```

### 6.4.1 Splitting a cross-lane feature (Claude as integrator)

Claude almost never writes both sides itself; it slices the feature along the API contract and delegates each side.

```bash
# 1. backend half → codex
BE_ID=$(aitask --format json task create \
  --title "BE: POST /api/projects/{id}/tasks/{tid}/checkpoint" \
  --target codex --skill backend-api \
  --output-contract "Handler + integration test + OpenAPI patch" \
  | jq -r '.taskId')

# 2. frontend half → gemini
FE_ID=$(aitask --format json task create \
  --title "FE: Checkpoint button on TaskDetail page" \
  --target gemini --skill frontend-ui \
  --output-contract "React component + Storybook story + a11y check" \
  | jq -r '.taskId')

# 3. integration task stays with Claude, parented on both
aitask task create \
  --title "Integrate checkpoint flow E2E" \
  --target claude --skill integration \
  --parent "$BE_ID" --output-contract "Playwright spec green; manual run notes in result.md"
# (the FE_ID dependency is captured in the description — keep it visible there)
```

### 6.5 Programmatic loop (JSON-friendly)

```bash
aitask --format json task current | jq '.task.requiredSkills'
aitask --format json memory search "decision openviking root" --refs-only | jq '.items[].uri'
```

## 7. Failure modes & how to react

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| `project_id is required` | Not in a bound repo | `aitask init --project <id>` or `cd` into the repo with `.aitask/project.md` |
| `no current task found` | No delegated task for this agent | `aitask task inbox`; if empty, stop and report. |
| `task <id> has no active run id` | Forgot `task start` | `aitask task start <id>` first |
| `bind code token is empty` / decode error | Wrong code format | Re-issue from console; valid forms: raw token, `agt_xxx:<token>`, `aitask-bind:<base64>` |
| `Backend unreachable` on `task current` | Network/backend down | CLI auto-falls back to `.aitask/current-task.md` cache; treat as read-only and do not submit |
| WebSocket close on `room watch` | Token expired or backend restart | Re-`auth bind`; re-run `room watch` |

Any unhandled error is rewrapped with hint metadata — read both stdout and stderr, do not silently retry.

## 8. Anti-patterns (do not do)

- ❌ Pull tasks by status filter and self-assign — tasks are delegated, period.
- ❌ Edit `.aitask/state/*.pb` by hand.
- ❌ Submit `task submit` with a stale `--run`; always let the CLI resolve via `task current` unless you know the run id.
- ❌ Dump full memory content into prompt context — use `memory search --refs-only` then read only what you need.
- ❌ Talk in chat instead of `room send` / `room ask` — coordination must be persisted.
- ❌ Bypass the CLI to call REST/RPC directly. The CLI maintains local state files; raw HTTP calls will desync.
- ❌ Cross lanes silently. Claude must not commit backend Go code or React UI on its own — split into `codex` / `gemini` child tasks per §1.1. Codex must not edit `fronted/` paths; Gemini must not edit `backend/` or migrations.

## 9. Quick smoke test

Use this when you suspect environment drift:

```bash
aitask --version
aitask whoami
aitask project info
aitask bootstrap --format brief
aitask context status --format brief
```

If all five succeed, the agent is fully wired.
