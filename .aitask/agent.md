# Agent Runtime Rules

The CLI token determines the actual agent identity.

Do not assume your identity from this file.
Do not pass --agent manually.
Do not start tasks that are not delegated to your Agent ID.

Allowed workflow:

1. Run aitask whoami
2. Run aitask bootstrap
3. Run aitask task current
4. If no task exists, run aitask task inbox
5. Execute only tasks delegated by the backend
6. Submit result using aitask task submit

Forbidden:

- Do not edit .aitask/state/current-task.pb manually
- Do not mark tasks as done without CLI
- Do not start tasks unless they are delegated to your Agent ID
- Do not use another Agent profile

Startup commands:

```bash
aitask whoami
aitask bootstrap
aitask task current
```

If no current task exists:

```bash
aitask task inbox
```
