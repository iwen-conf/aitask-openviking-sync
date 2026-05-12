# OpenViking Integration

OpenViking is AITask's long-term memory and semantic retrieval backend, not its queue, status store, or scheduler.

## Boundary

OpenViking owns semantic memory:

- long-lived project memory;
- semantic search;
- context recall;
- thread / project / agent summaries;
- reusable resources and skills;
- cross-Agent shared knowledge.

OpenViking does not own runtime state:

- event routing;
- message queue semantics;
- ack / handled / failed / skipped state;
- retry counters;
- cursors;
- Agent wake routing;
- real-time notification.

The rule is:

```text
state stays in state.db
meaning goes to OpenViking
```

## Write Candidates

`aitask-worker` may sync only high-value semantic content:

- mention body;
- delegated task description;
- Agent reply body;
- task result;
- failure reason after sanitization;
- thread / project / agent summary;
- key decisions and important context changes.

It should skip:

- heartbeat and daemon lifecycle noise;
- duplicate notifications;
- low-value debug logs;
- raw long tool output;
- meaningless stdout.

## Configuration

AITask can use project-level OpenViking settings from the backend:

- Server URL;
- API Key;
- Namespace;
- Workspace ID;
- memory write enable switch;
- auto-sync enable switch.

The CLI should not overwrite `~/.openviking/ovcli.conf`. OpenViking's own CLI config remains independent:

```text
~/.openviking/ovcli.conf
{
  "url": "http://localhost:1933",
  "api_key": "..."
}
```

## AITask Commands

```bash
# Existing backend-proxied memory commands
aitask memory search "wake-and-inject"
aitask memory read viking://aitask/projects/<project>/memory/...
aitask memory write --target summary --title "..." --from result.md

# Context budget and handoff commands
aitask context status
aitask context compact
aitask context handoff prepare
```

Direct OpenViking CLI examples:

```bash
openviking observer system
openviking add-resource <url-or-file>
openviking ls viking://resources
openviking find "what is openviking"
```

## Integration Flow

### Worker Sync

```text
1. SELECT memory_sync WHERE status='pending' LIMIT N
2. JOIN events to render compact semantic content
3. POST through backend memory write API
4. success -> memory_sync.status='synced' + openviking_id
5. failure -> memory_sync.status='failed' + retry_count++ + last_error
```

### Retrieval

```text
Agent/task needs context
  -> use backend-proxied memory search/read
  -> combine references with state.db metadata
  -> include concise refs in prompt or handoff
```

### Summary Writeback

```text
aitask-worker updates state.db.summaries
  -> writes summary memory through backend
  -> records memory_id or URI in summaries.memory_id
```

## Failure Policy

- Network unavailable: mark sync failed, retry later.
- 401 / 403: surface configuration error; do not spin hot retries.
- 429: exponential backoff with jitter.
- Timeout: keep local state, retry next worker cycle.
- Retrieval failure: caller must degrade to no-recall prompt.

OpenViking failures must not block `events.ndjson` ingest, inbox queries, or status updates.

## Related Files

- `integrations/openviking/skills/SKILL.md` — component skill contract.
- `aitask-worker` skill — <https://github.com/iwen-conf/aitask-cli-skill/blob/main/skills/aitask-worker/SKILL.md>.
- `cli/internal/state/README.md` — `state.db` tables for `memory_sync` and `summaries`.
- `core/internal/service/openviking/` — backend client and project settings implementation.
