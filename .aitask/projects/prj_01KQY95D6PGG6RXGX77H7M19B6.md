# AI Task Project

project_id: prj_01KQY95D6PGG6RXGX77H7M19B6
project_name: prj_01KQY95D6PGG6RXGX77H7M19B6
openviking_root: viking://aitask/projects/prj_01KQY95D6PGG6RXGX77H7M19B6
room_enabled: true

# Redline: never store agent token in this file.
# Token must be kept in system keychain or ~/.aitask/credentials only.

## Rule

This repository is controlled by the aitask CLI.

AI agents must not rely on chat history.

Before doing any work, run:

```bash
aitask bootstrap
```

Then run:

```bash
aitask task current
```

If there is no current task, inspect delegated tasks:

```bash
aitask task inbox
```

All task results must be submitted through:

```bash
aitask task submit
```
