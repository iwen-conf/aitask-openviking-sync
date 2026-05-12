# Skill: Codex CLI 集成

## 定位

Codex CLI 是 OpenAI / OpenViking 生态中的编码 Agent CLI。
本目录承载 Codex CLI 与 AITask 的 hook 集成与 agent-watch runner。
它属于 Context Injection Mode 的"Codex 一侧适配"，并为 Mailbox Worker Mode 的 `--wake codex` 提供 runner 配方。

## Codex CLI 的 hook 能力

| Hook | 是否可用 | 用途 |
| --- | --- | --- |
| `SessionStart` | ✅ | 启动时注入最近 actionable events |
| `UserPromptSubmit` | ✅ | 每轮 prompt 前注入增量事件 |
| `BeforeAgent` | ✅ | 与上一项功能可替换 |

> 旧版本 Codex 需要在 `~/.codex/config.toml` 中设置 `[features] codex_hooks = true`；新版本默认开启。

## 负责什么

- 安装 / 维护 `scripts/aitask-hooks/codex-session-start.sh`。
- 安装 / 维护 `scripts/aitask-hooks/codex-prompt-submit.sh`（per-turn 增量）。
- 提供 `aitask watch --agent codex --wake` 的 runner 入口（一次性 `codex exec`）。
- 在 prompt 渲染时附带 Codex 偏好（如倾向 unified diff）。

## 不负责什么

- Codex CLI 的安装与升级。
- 服务端事件订阅。
- inbox / 状态机。
- OpenViking 同步。
- stdin 注入正在运行的 Codex REPL（**禁止**）。

## 输入

| 来源 | 内容 |
| --- | --- |
| `~/.aitask/events.ndjson` | hook 注入数据源 |
| `~/.aitask/hook-state/codex-prompt-offset` | per-turn 增量 byte offset |
| `~/.codex/hooks.json` | hook 配置（安装时合并） |

## 输出

- SessionStart / UserPromptSubmit 时输出 `{hookSpecificOutput: {hookEventName, additionalContext}}`。
- runner 模式下：`codex exec "$prompt"` 一次性执行，stdout 给 agent-watch。

## 核心命令

```bash
# 安装两个 hook
scripts/aitask-hooks/install.sh codex

# Mailbox 模式下作为 runner
aitask watch --agent codex --wake
# 实际等价于：
codex exec "<rendered prompt>"
```

## 状态文件

- `~/.aitask/hook-state/codex-prompt-offset`（每次 UserPromptSubmit 推进）。
- `~/.codex/hooks.json`（hook 配置）。

## 与其他组件的关系

- 上游：`aitask-watch` 提供事件源。
- 同侪：与 Claude / Gemini 集成共享 `scripts/aitask-hooks/lib.sh`。
- runner：被 `aitask-agent-watch --wake codex` 调用。
- 在 Phase 2/3/4 实施时，Codex 常作为 unified-diff 生成方被主 Agent 委托。

## 常见流程

### 1. SessionStart 注入

```text
1. codex-session-start.sh 启动
2. aitask_ensure_watch_daemon → tmux 自启
3. aitask_recent_events 10
4. emit hookSpecificOutput
```

### 2. UserPromptSubmit 增量注入

```text
1. codex-prompt-submit.sh 触发
2. aitask_new_events_since_last "codex"（按 byte offset 取增量）
3. 仅 mention / task_delegated 写入 additionalContext
4. 推进 offset
5. 无新事件时输出空 additionalContext，避免噪音
```

### 3. 作为 runner 被唤醒

```text
agent-watch
  → aitask render-prompt --event $id --agent codex
  → codex exec "$prompt"
  → stdout 作为结果回写
  → aitask done / fail / skip
```

## 失败与重试策略

- 旧版 Codex 未启用 hook：hook 静默不执行，不影响 CLI；用户可在 `config.toml` 启用。
- offset 文件丢失：自动重置为当前 EOF（不回放历史）。
- `codex` 缺失：runner 失败并记 `last_error="codex not found"`。
- runner 超时：杀进程并标 failed。

## Agent 使用注意事项

- per-turn hook 必须**只注入新事件**，否则每轮 prompt 重复历史 → 上下文膨胀。
- Codex 偏好 unified diff，prompt renderer 在指令段建议加："Return changes as unified diff."。
- Codex 沙箱默认禁止写文件（用户的 CLAUDE.md 全局协议）；agent-watch 在调用前确认 prompt 不要求 Codex 写盘，结果以 diff 文本返回。
- 不要让 Codex 在 hook 里递归调用 aitask-watch 之外的写命令。
- 测试覆盖必须包含：
  1. 首次启动 offset 自动 arm 在 EOF；
  2. NDJSON 截断后 offset 安全重置；
  3. `codex` 缺失时 runner 友好失败。
