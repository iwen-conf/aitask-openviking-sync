# OpenViking Integration

OpenViking is AITask's long-term semantic memory and retrieval backend.

```text
state stays in state.db
meaning goes to OpenViking
```

OpenViking owns project memory, semantic search, context recall, summaries, skills, resources, and cross-Agent knowledge. It does not own event routing, queue state, ack/handled/failed status, retry counters, cursors, wake routing, or task authority.

## Configuration

Project OpenViking settings are managed by the backend and settings page:

- `serverUrl`
- `apiKey`
- `namespace`
- `workspaceId`
- `enableMemoryWrite`
- `enableAutoSync`

`enableMemoryWrite=false` disables all memory writes for the project.

`enableAutoSync=false` disables automatic background writes such as task summaries, room summaries, context handoff sync, and CLI worker sync. Explicit manual writes through `aitask memory write` are still allowed when `enableMemoryWrite=true`.

AITask does not overwrite OpenViking's own CLI config at `~/.openviking/ovcli.conf`.

## Search Order

AITask search prefers backend-proxied OpenViking memory search:

```bash
aitask search "wake-and-inject"
aitask memory search "wake-and-inject"
```

If `aitask search` receives no memory hits, or OpenViking is temporarily unavailable, it falls back to local `rg` file search and labels the JSON result with `"fallback": true`.

## Context Recall

Event and thread scoped context can be rendered from local state:

```bash
aitask context --event <event_id>
aitask context event <event_id>
aitask context --thread <thread_id>
aitask context thread <thread_id>
```

Event context includes local event metadata, body, thread summary when available, and best-effort OpenViking recall for the event id.

## Automatic Memory Sync

`aitask worker --memory openviking` and `aitask worker --memory backend` are equivalent. Both sync pending semantic rows from `state.db.memory_sync` through the backend memory API.

Synced candidates include mentions, delegated tasks, Agent replies, task results, failures, handoffs, summaries, decisions, and important room/project context changes. Low-value daemon, heartbeat, ready, and debug noise should stay out of memory.

`aitask watch --agent <name> --exec|--wake` stores successful runner stdout as a local `task_done` event and enqueues it for worker sync. The backend task submit path remains authoritative for task state.
