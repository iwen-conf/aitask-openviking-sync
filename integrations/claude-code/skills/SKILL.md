# Skill: Claude Code 集成

## 定位

Claude Code 是 Anthropic 官方 CLI。
本目录承载 Claude Code 与 AITask 的 hook 集成、Monitor 推荐用法以及 agent-watch runner。
它属于 Context Injection Mode 的"Claude 一侧适配"，并为 Mailbox Worker Mode 的 `--wake claude-code` 提供 runner 配方。

## Claude Code 的 hook 能力

| Hook | 是否可用 | 用途 |
| --- | --- | --- |
| `SessionStart` | ✅ | 启动时注入最近 actionable events |
| per-turn（如 UserPromptSubmit / BeforeAgent） | ❌ 无等价 | 改用 Monitor 工具 `tail -F` |
| `Stop` / `Notification` 等 | ✅ | 与本集成无强依赖 |

## 负责什么

- 安装 / 维护 `scripts/aitask-hooks/claude-session-start.sh`。
- 在会话内通过 Monitor 工具实现近实时事件推送（替代 per-turn hook）。
- 提供 `aitask watch --agent claude-code --wake` 的 runner 入口（一次性 `claude -p`）。
- 提供"Claude Code 行为差异"的最小说明，帮助 prompt renderer 适配。

## 不负责什么

- Claude Code 自身的安装与升级。
- 服务端事件订阅（→ aitask-watch）。
- inbox 状态机（→ aitask-inbox）。
- OpenViking 同步（→ aitask-worker）。
- 接管正在运行的 Claude Code REPL（**禁止** stdin 注入）。

## 输入

| 来源 | 内容 |
| --- | --- |
| `~/.aitask/events.ndjson` | SessionStart 注入数据源 |
| `~/.claude/settings.json` | 安装时合并 hook 配置 |
| Monitor 工具 | per-turn 实时推送 |

## 输出

- SessionStart 时输出 `{hookSpecificOutput: {hookEventName: "SessionStart", additionalContext: "..."}}`。
- runner 模式下：`claude -p "$prompt"` 一次性执行并退出，stdout 给 agent-watch。

## 核心命令

```bash
# 安装 SessionStart hook
scripts/aitask-hooks/install.sh claude

# 在会话内拿到近实时事件（替代 per-turn hook）
Monitor: tail -F -n 0 ~/.aitask/events.ndjson \
  | jq -c 'select(.kind == "mention" or .kind == "task_delegated")'

# Mailbox 模式下作为 runner 被调用
aitask watch --agent claude-code --wake
# 实际等价于：
claude -p "<rendered prompt>"
```

## 状态文件

- `~/.aitask/hook-state/claude-prompt-offset`（保留供未来 per-turn hook 使用，目前 SessionStart 不消费）。
- `~/.claude/settings.json`（hooks 配置）。

## 与其他组件的关系

- 上游：`aitask-watch` 提供事件源。
- 同侪：与 Codex / Gemini 集成共享 `scripts/aitask-hooks/lib.sh`。
- 下游：会话内人类用户。
- runner：被 `aitask-agent-watch --wake claude-code` 调用。

## 常见流程

### 1. SessionStart 注入

```text
1. claude-session-start.sh 启动
2. aitask_ensure_watch_daemon → tmux 自启
3. aitask_recent_events 10 → tail 10 条 actionable
4. emit hookSpecificOutput
5. 同时附一行 "Monitor: tail -F ..." 供用户在会话中开 Monitor
```

### 2. 在会话中获取增量

Claude Code 没有 per-turn hook。让用户（或自动化模板）开一个 Monitor：

```bash
tail -F -n 0 ~/.aitask/events.ndjson | jq -c 'select(.kind=="mention" or .kind=="task_delegated")'
```

每行 stdout 即一条事件通知。

### 3. 作为 runner 被唤醒

```text
agent-watch
  → aitask render-prompt --event $id --agent claude-code
  → echo "$prompt" | claude -p
  → 捕获 stdout 作为结果
  → aitask done / fail / skip
```

## 失败与重试策略

- Hook 脚本失败：Claude Code 仍能正常启动；事件未注入将由 Monitor 兜底。
- `claude` CLI 缺失：runner 模式直接 `aitask fail`，提示用户安装。
- runner 超时：由 agent-watch 杀进程并标 failed。

## Agent 使用注意事项

- **不要** 用 tmux send-keys 把 prompt 注入到正在运行的 Claude Code 会话里。
- 使用 `claude -p` 的一次性 headless 模式作为 runner，避免破坏当前会话状态。
- SessionStart 的 `additionalContext` 必须保持纯文本/Markdown，避免大段 JSON 让模型分心。
- Monitor 提示应该只引导 tail，不应在第一轮写入大量历史事件（`-n 0`）。
- 测试覆盖必须包含：
  1. tmux 不可用时 hook 仍输出 hint；
  2. NDJSON 缺失时 hook 不报错；
  3. `claude` CLI 缺失时 runner 优雅失败。
