# Skill: Gemini CLI 集成

## 定位

Gemini CLI 是 Google 的编码 Agent CLI。
本目录承载 Gemini CLI 与 AITask 的 hook 集成与 agent-watch runner。
它属于 Context Injection Mode 的"Gemini 一侧适配"，并为 Mailbox Worker Mode 的 `--wake gemini` 提供 runner 配方。

## Gemini CLI 的 hook 能力

| Hook | 是否可用 | 用途 |
| --- | --- | --- |
| `SessionStart` | ✅ | 启动时注入最近 actionable events |
| `BeforeAgent` | ✅ | 每次 Agent 规划前注入增量事件 |

## 负责什么

- 安装 / 维护 `scripts/aitask-hooks/gemini-session-start.sh`。
- 安装 / 维护 `scripts/aitask-hooks/gemini-before-agent.sh`（per-turn 增量）。
- 提供 `aitask watch --agent gemini --wake` 的 runner 入口（一次性 `gemini "$prompt"`）。
- 在 prompt 渲染时附带 Gemini 偏好（如倾向前端任务）。

## 不负责什么

- Gemini CLI 的安装与升级。
- 服务端事件订阅。
- inbox / 状态机。
- OpenViking 同步。
- stdin 注入正在运行的 Gemini REPL（**禁止**）。

## 输入

| 来源 | 内容 |
| --- | --- |
| `~/.aitask/events.ndjson` | hook 注入数据源 |
| `~/.aitask/hook-state/gemini-prompt-offset` | per-turn 增量 byte offset |
| `~/.gemini/settings.json` | hook 配置 |

## 输出

- SessionStart / BeforeAgent 输出 `{hookSpecificOutput: {hookEventName, additionalContext}}`。
- runner 模式下：`gemini "$prompt"` 一次性执行，stdout 给 agent-watch。

## 核心命令

```bash
# 安装两个 hook
scripts/aitask-hooks/install.sh gemini

# Mailbox 模式下作为 runner
aitask watch --agent gemini --wake
# 实际等价于：
gemini "<rendered prompt>"
```

## 状态文件

- `~/.aitask/hook-state/gemini-prompt-offset`。
- `~/.gemini/settings.json`。

## 与其他组件的关系

- 上游：`aitask-watch` 提供事件源。
- 同侪：与 Claude / Codex 集成共享 `scripts/aitask-hooks/lib.sh`。
- runner：被 `aitask-agent-watch --wake gemini` 调用。
- 通常承担前端类任务（按 CLAUDE.md 中模型路由规则）。

## 常见流程

### 1. SessionStart 注入

```text
1. gemini-session-start.sh 启动
2. aitask_ensure_watch_daemon → tmux 自启
3. aitask_recent_events 10
4. emit hookSpecificOutput
```

### 2. BeforeAgent 增量注入

```text
1. gemini-before-agent.sh 触发
2. aitask_new_events_since_last "gemini"
3. 仅 mention / task_delegated 写入 additionalContext
4. 推进 offset
```

### 3. 作为 runner 被唤醒

```text
agent-watch
  → aitask render-prompt --event $id --agent gemini
  → gemini "$prompt"
  → stdout 作为结果回写
  → aitask done / fail / skip
```

## 失败与重试策略

- offset 文件丢失：自动重置为当前 EOF。
- `gemini` 缺失：runner 失败并记 `last_error="gemini not found"`。
- runner 超时：杀进程并标 failed。
- Gemini API 限流（runner 端的 429）：fallback skip，提示稍后重试。

## Agent 使用注意事项

- per-turn hook 必须只注入新事件。
- Gemini 在长对话中容易丢上下文，prompt renderer 应把召回内容放在前面、指令放在后面。
- Gemini 多模态能力较强，但 mailbox watcher 第一版只走文本。
- 不要让 Gemini 在 hook 里写盘（同 Codex 沙箱原则）。
- 测试覆盖必须包含：
  1. 首次启动 offset 自动 arm 在 EOF；
  2. NDJSON 截断后 offset 安全重置；
  3. `gemini` 缺失时 runner 友好失败。
