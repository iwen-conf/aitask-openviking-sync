# Local Runtime

AITask's local runtime uses two append/persist layers:

- `~/.aitask/events.ndjson`: append-only event stream from `aitask-watch`;
- `~/.aitask/state.db`: SQLite authority for inbox rows, status, cursors, summaries, and memory sync state.

## Files

```text
~/.aitask/events.ndjson
~/.aitask/state.db
~/.aitask/runtime/worker.lock
~/.aitask/runtime/agent-watch/<agent>.lock
```

`events.ndjson` can exist without `state.db`. Read commands such as `aitask inbox`, `aitask latest`, `aitask thread`, and `aitask context` can build an in-memory index when `state.db` is missing.

Status-changing commands require `state.db`.

## Worker

```bash
aitask worker --once --memory none
aitask worker --once --memory openviking
aitask worker --daemon --memory openviking
aitask worker --backfill-since 2026-05-08T00:00:00Z --limit 100
```

The worker:

- ingests new NDJSON events into `state.db.events`;
- routes agent-specific messages into `agent_inbox`;
- routes global/project messages into `global_feed`;
- enqueues high-value semantic events into `memory_sync`;
- refreshes lightweight thread summaries;
- optionally syncs pending memory rows through the backend.

## Agent Watch

```bash
aitask watch --agent codex --once --dry-run
aitask watch --agent codex --once --exec ./handler
aitask watch --agent gemini --wake gemini
```

`--dry-run` renders the prompt only. `--exec` sends the prompt to the executable on stdin. `--wake` starts a supported Agent CLI.

On success, runner stdout is recorded as a local `task_done` event and queued in `memory_sync`. On failure, inbox status is marked failed and retry counters stay in `state.db`.

## Recovery

OpenViking can be unavailable without breaking local inbox reads. Pending rows remain in `memory_sync` and can be retried by running the worker again.

If the worker or agent watcher is already running, lock files prevent duplicate processing. Removing lock files should only be done after confirming no matching process is alive.
