# AITask Runtime Modes

AITask has two local collaboration modes. They can run together, but they own different responsibilities.

## Context Injection Mode

Context Injection Mode is the existing lightweight path.

```text
aitask events / WebSocket
  -> aitask-watch
  -> ~/.aitask/events.ndjson
  -> Claude / Codex / Gemini hooks
  -> prompt context
```

Use it when a human is actively using an Agent and the Agent only needs recent events injected into its current session.

Owned by this mode:

- subscribe to project events;
- append `events.ndjson`;
- notify locally;
- provide hook input for Agent startup or per-turn context.

Not owned by this mode:

- inbox status;
- retry counters;
- OpenViking sync;
- task scheduling;
- automatic task authority.

## Mailbox Worker Mode

Mailbox Worker Mode adds a local state index and optional semantic memory sync.

```text
~/.aitask/events.ndjson
  -> aitask worker
  -> ~/.aitask/state.db
  -> inbox / latest / thread / summary / context
  -> OpenViking memory sync
```

Use it when Agents need their own inbox, global feed, ack/done/fail/skip status, retry accounting, summaries, and OpenViking memory persistence.

Key commands:

```bash
aitask worker --once --memory openviking
aitask worker --daemon --memory openviking
aitask inbox --agent codex
aitask latest
aitask context --thread <thread_id>
aitask context --event <event_id>
```

## Agent Runner Mode

`aitask watch --agent <name>` consumes local inbox rows and can execute a runner.

```text
state.db agent_inbox
  -> aitask watch --agent <name> --exec|--wake
  -> runner stdout
  -> state.db events + memory_sync
  -> aitask worker sync
  -> OpenViking
```

Runner output is persisted as semantic task result content. It is not treated as the source of truth for task lifecycle authority; task submission and review remain backend-owned.
