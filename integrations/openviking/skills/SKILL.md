# Skill: openviking 集成

## 定位

OpenViking 是 AITask 的长期记忆与检索后端（Context Database）。
本目录承载 AITask 与 OpenViking 之间的所有适配代码、配置约定与 CLI 调用。
它不承担事件路由、状态机、唤醒调度。

## 负责什么

- 把 worker 选出的语义子集写入 OpenViking（mention 正文 / task 描述 / 回复 / 结果 / summary）。
- 提供 `aitask search` / `aitask context` / `aitask summary` 的检索能力。
- 维护 OpenViking 连接配置（URL / API Key / Account / Workspace / Project ID）。
- 暴露设置面板字段（与 `Openviking接入真实情况.md` 中的需求对齐）。
- 在 Agent 端实现"prefer OpenViking, fallback rg / ace-tool / cocoindex-code"的检索路由。
- 屏蔽 OpenViking SDK / REST 细节，给上层提供稳定接口。

## 不负责什么

- 事件总线 / 消息队列。
- 任务调度。
- ack / handled / retry / cursor。
- 唤醒 Agent。
- 把状态类数据写入 OpenViking。
- 替代 events.ndjson 作为事件源。

## 输入

| 来源 | 内容 |
| --- | --- |
| `~/.openviking/ovcli.conf` | OpenViking CLI 配置（URL / api_key） |
| `~/.aitask/config.json` | AITask 侧的 OpenViking 启用开关 + Account / Workspace / Project |
| 环境变量 | `OPENVIKING_CLI_CONFIG_FILE`、`AITASK_OPENVIKING_ENABLE` |
| 上层调用 | worker 同步、CLI 检索请求 |

## 输出

- OpenViking memory / resource 写入结果（`memory_id`）。
- 检索结果（结构化片段 + 来源 reference）。
- 失败错误码（透传到 `state.db.memory_sync.last_error`）。

## 核心命令（外部）

```bash
# OpenViking CLI 自身
openviking observer system
openviking add-resource <url-or-file>
openviking ls viking://resources
openviking find "what is openviking"

# AITask 提供的封装
aitask search "wake-and-inject 方案"
aitask context --event <event_id>
aitask context --thread <thread_id>
aitask summary --project <name>
aitask summary --thread <thread_id>
aitask summary --agent <name>

# 复用 OpenViking CLI 的 ovcli.conf
aitask openviking config show                       # 查看会加载哪个 conf 与字段（API key 自动掩码）
aitask openviking config show --path /path/to.conf  # 指定 conf 文件
aitask openviking config import --dry-run           # 预览 PUT body，不打后端
aitask openviking config import --project <id>      # 把 ovcli.conf 写入项目 OpenViking 设置
```

`openviking config show / import` 默认按上游 OpenViking CLI 约定解析 `~/.openviking/ovcli.conf`，并尊重 `OPENVIKING_CLI_CONFIG_FILE` 环境变量覆盖。`import` 把 `url` / `api_key` / `workspace_id` / `namespace` 推送到后端 `PUT /api/projects/{id}/openviking/settings`，等价于在前端设置面板手动填写。

## 状态文件

- `~/.openviking/ovcli.conf`：OpenViking 自身配置，AITask 不直接覆盖。
- `~/.aitask/config.json` 中的 OpenViking 段：

```json
{
  "openviking": {
    "enabled": true,
    "server_url": "http://localhost:1933",
    "api_key": "...",
    "account": "...",
    "workspace": "...",
    "project": "...",
    "auto_sync": true
  }
}
```

- `state.db.memory_sync` 与 `state.db.summaries.memory_id`。

## 与其他组件的关系

- 被 `aitask-worker` 调用做异步同步。
- 被 `aitask inbox/search/context/summary` 调用做检索。
- 被 `aitask-agent-watch` 在组装 prompt 前调用。
- OpenViking 不主动回调 AITask；所有交互由 AITask 发起。

## 常见流程

### 1. worker 同步

```text
1. SELECT memory_sync WHERE status='pending' LIMIT N
2. for each: 调用 OpenViking SDK / REST 写入 viking://aitask/projects/<project>/...
3. 成功 → status='synced' + openviking_id
4. 失败 → status='failed' + retry_count++ + last_error
```

### 2. 检索召回

```text
aitask context --event <id>
  → query OpenViking by tags / metadata / similarity
  → 返回 top-k 片段
  → 同时附 state.db 中该事件的本地元数据（thread / from / created_at）
```

### 3. summary 写回

```text
aitask-worker 生成 summary
  → 写 state.db.summaries
  → 调用 OpenViking 写 memory（带 scope 标签）
  → memory_id 落到 summaries.memory_id
```

## 失败与重试策略

- 网络不可达：worker 标 `failed`，下一轮 `pending` 自动重试。
- 401 / 403：触发"配置缺失"错误码，不重试，提示用户检查 API Key。
- 429 限流：指数退避 + jitter；retry_count 单独记。
- 写入超时：fallback `failed`，下次 worker 周期重试；不丢数据。
- 检索失败：上层（agent-watch / CLI）必须能 graceful degrade（无召回也能渲染 prompt）。

## Agent 使用注意事项

- 不要把 cursor / ack / handled 写入 OpenViking。
- 不要把"超长工具原始输出"原文塞进去——先做摘要或截断。
- 不要把高频心跳事件写入。
- 不要把"与 Agent 无关的 debug log"写入。
- 设置面板必须区分"启用 OpenViking"与"启用任务结束自动同步"两个开关，避免一刀切。
- 测试覆盖必须包含：
  1. OpenViking 不可达时 worker 仍能 ingest；
  2. 401 时不会重试到限流；
  3. 检索失败 fallback 无召回 prompt；
  4. summary 写回的 memory_id 正确落库。
