# Skill: aitask-watch

## 定位

`aitask-watch` 是 AITask 本地事件流守护进程。
它把服务端 actionable 事件流订阅到本地 `~/.aitask/events.ndjson`，
并通过 SessionStart / per-turn hook 把事件注入 Claude Code / Codex CLI / Gemini CLI 的会话上下文。

它属于 Context Injection Mode（见 `../../../cli/internal/cli/aitask-watch.md`）。
它不属于 Mailbox Worker Mode，也不是"自动应答系统"。

## 负责什么

- 通过 `aitask events` 经 WebSocket 订阅服务端项目事件流。
- 将事件追加写入 `~/.aitask/events.ndjson`（NDJSON，每行一个 JSON 对象）。
- 在 macOS / Linux 发系统通知。
- 维持 tmux daemon（默认 session 名 `aitask-watch`）。
- 给 `scripts/aitask-hooks/*.sh` 提供共同事件源。
- 给 Claude Code 的 SessionStart、Codex 的 SessionStart + UserPromptSubmit、Gemini 的 SessionStart + BeforeAgent 提供注入数据。

## 不负责什么

- OpenViking 同步 → 由 `aitask-worker` 负责。
- inbox 状态机（unread / acked / handled / failed） → 由 `aitask-inbox` + `state.db` 负责。
- ack / handled / failed / retry → 由 `aitask-inbox` + `aitask-agent-watch` 负责。
- Agent 自动唤醒 → 由 `aitask-agent-watch` 负责。
- summary 生成 → 由 `aitask-worker` 负责。
- 任务状态判断（哪个 Agent 该处理）→ 由 worker 路由 + agent-watch 决定。
- 复杂任务编排、跨机调度。

## 输入

| 来源 | 内容 |
| --- | --- |
| 服务端 WebSocket | `mention` / `task_delegated` / `broadcast` / `task_done` 等 actionable event |
| 用户配置 | `~/.aitask/config.json`、active profile、token |
| 环境变量 | `AITASK_EVENTS_FILE`、`AITASK_WATCH_TMUX`、`AITASK_WATCH_BIN`、`AITASK_WATCH_ARGS` |

## 输出

| 形式 | 内容 |
| --- | --- |
| `~/.aitask/events.ndjson` | 每行一个 JSON 事件，append-only |
| OS 系统通知 | mention / task_delegated 的提示 |
| stdout（可选） | 调试时打印事件 |

## 核心命令

```bash
# 一次性流式输出
aitask events

# 守护方式（tmux 内运行）
tmux new -ds aitask-watch "aitask-watch --notify auto --stdout=false"

# 各 Agent hook 安装入口
scripts/aitask-hooks/install.sh           # 全部
scripts/aitask-hooks/install.sh claude    # 仅 Claude Code
scripts/aitask-hooks/install.sh codex     # 仅 Codex CLI
scripts/aitask-hooks/install.sh gemini    # 仅 Gemini CLI
```

## 状态文件

- `~/.aitask/events.ndjson` — 事件流。
- `~/.aitask/hook-state/<agent>-prompt-offset` — 各 hook 私有的 byte-offset。
- `state.db` 不归 aitask-watch 管。

## 与其他组件的关系

- 上游：服务端 actionable events stream。
- 下游：
  - 各 Agent hook（直接读 NDJSON）。
  - `aitask-worker`（消费 NDJSON 写入 state.db）。
  - 人类查看 / `tail -F`。
- 单点写入：events.ndjson 只能由 aitask-watch 写。

## 常见流程

### 1. SessionStart 注入

```text
hook 触发
  → 检查 tmux session 存在性
  → 缺失则自动 new -ds 启动 aitask-watch
  → tail 最近 N 条 actionable event
  → emit hookSpecificOutput.additionalContext
```

### 2. 每轮增量注入（Codex / Gemini）

```text
hook 触发
  → 读取 hook-state offset
  → 读取 events.ndjson 增量
  → 过滤 kind in (mention, task_delegated)
  → emit additionalContext
  → 推进 offset
```

### 3. Claude Code 在会话中

Claude Code 没有 per-turn hook。建议：

```bash
Monitor: tail -F -n 0 ~/.aitask/events.ndjson | jq -c 'select(.kind=="mention" or .kind=="task_delegated")'
```

## 失败与重试策略

- WebSocket 断线：`aitask events` 自带重连；外层 tmux daemon 进程崩溃由用户 hook 自动 `tmux new -ds` 重启。
- NDJSON 写入失败（磁盘满）：进程退出；tmux 重启后会自动重连，cursor 由服务端侧给出 `since=`。
- hook 读 NDJSON 时遇到截断：使用 byte-offset 自检 `size < last_offset` 时重置。

## Agent 使用注意事项

- 不要把 OpenViking 状态、任务 ack、retry_count 写进 events.ndjson。
- 不要让任何 Agent 直接写 events.ndjson，全部通过 aitask-watch。
- 不要把 aitask-watch 当作"自动应答系统"——它只是事件源。
- 修改 hook 脚本时，必须保持 `additionalContext` 的字段名与 JSON 结构稳定（Claude / Codex / Gemini 都依赖）。
- 测试覆盖必须包含：
  1. NDJSON 不存在；
  2. NDJSON 被截断；
  3. tmux session 已存在；
  4. tmux 不可用；
  5. AITask CLI suite 未安装，导致 `aitask-watch` 不在 PATH。
