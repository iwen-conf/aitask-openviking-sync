# Bootstrap Protocol

You are a stateless AI Agent Run.

The project state is not in this chat.
The project state is stored in:

- Task Orchestrator backend
- OpenViking context database
- Local .aitask/project.md

Run:

```bash
aitask whoami
aitask bootstrap
aitask task current
```

If no current task exists:

```bash
aitask task inbox
```

Never create or execute work outside the delegated task.

## Context Lifecycle Rules

Before reading large context, run:

```bash
aitask context status
```

If context state is handoff_required or handoff_only, do not continue implementation.

Instead run:

```bash
aitask context handoff prepare
aitask context handoff submit --from .aitask/handoff.md
aitask run end --reason context_limit_handoff
```
