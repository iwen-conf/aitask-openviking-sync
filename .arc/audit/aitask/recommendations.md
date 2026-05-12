# Recommendations — aitask 内网单机就绪度修复路线

> **部署假设**（信息源：`docs/API/README.md`）：
> - 不做用户管理（无 login / register / refresh / logout / users API）。
> - Web Console 按内网受信控制台处理；公网由外层网关负责，不在本系统内增加用户体系。
> - docker-compose 单机部署，**不上 Kubernetes**。
> - 当前目标：**先把业务功能跑通**。

> 优先级语义：**P0 = 跑通业务前必修**；**P1 = 跑通后稳态前必收敛**；**P2 = 演进项**。
> 每条都给出 **Quick Fix（可直接拷贝执行）**。

---

## ❌ 已撤销条目（不再适用）

| 原编号 | 原结论 | 撤销理由 |
|---|---|---|
| ~~P0-1 无 Auth 自动 operator 放行~~ | **本来就是设计意图**（`docs/API/README.md` 第 7、11 行已声明）。Web Console 操作者通过 `operatorLabel` 标记，不作为权限主体；`RequireProjectAccess` 仅对 agent 身份做 ACL 是合理设计 |
| ~~P0-3 CORS 没启用~~ | 内网同源部署（前端 nginx + `runtime-config.js` 拼接同源后端地址）无跨域，不需要 |
| ~~P1-5 k8s manifest / 备份 / 回滚预案~~ | docker-compose 单机即目标形态 |
| ~~P1-6 前端 Authorization 头注入路径~~ | 无用户态没有该通道 |
| ~~P2-1 配置中心化（vault/sops）~~ | 内网单机过度工程 |
| ~~P2-3 SLI/SLO 定义~~ | 跑通后再议 |

---

## P0 —— 跑通业务前必修（精简版，预计 2-3 天）

### P0-A 启动时拒绝默认密钥
**证据**：`backend/internal/config/config.go:23,18,21,405-410`，`docker-compose.yml` 默认值。
**风险**：即便内网部署，把 `AGENT_TOKEN_SECRET=dev_only_change_me` 等默认值带进部署环境变量是事故源（多人协作 / 测试环境向上漂移 / 无意识复用）。
**Quick Fix**：在 `Config.Validate()` 末尾加：
```go
if c.IsProduction() {
    forbidden := map[string]string{
        "AGENT_TOKEN_SECRET": defaultAgentSecret,
        "POSTGRES_PASSWORD":  defaultPostgresPass,
        "DRAGONFLY_PASSWORD": defaultDragonflyPass,
    }
    for k, v := range forbidden {
        if got := envString(k, v); got == v {
            return fmt.Errorf("%s must be overridden when APP_ENV=production", k)
        }
    }
}
```
配套：`docker-compose.yml` 把 `${VAR:-dev_*}` 兜底改为 `${VAR:?must set}` 强制非空。
**注意**：production 模式下 `sslmode=disable` 暂不强制（内网容器互联可接受）；如未来要把 Postgres 拉出 compose 网络再加。

### P0-B RequestID + 结构化访问日志
**证据**：`backend/internal/http/router.go:35-37` 仅挂了 `gin.Recovery()`。
**风险**：跑通业务时**没有这两个就根本看不到哪条请求挂在哪里**——状态机分支多，光看 handler 日志连不起调用链。
**Quick Fix**：注入两个轻量中间件，不引新依赖，靠 `pkg/ids` 已有的 ULID 生成器：
```go
r.Use(requestIDMiddleware())  // 生成 ULID 写入 context 与 X-Request-Id 响应头
r.Use(accessLogMiddleware(slog.Default()))
//   每条 method / path / status / latency_ms / request_id / identity_kind / identity_label
```
跑通业务剧本时可以用 `request_id` 串起 HTTP→Worker→OpenViking 全链路日志。

### P0-C 工作树 55 个 in-flight 文件分组提交
**证据**：`git status` / `git diff --stat` 显示 +2268/-641，含 `main.go / router.go / 核心 handlers`。
**风险**：任何后续修复都建立在漂移的地基上。
**Quick Fix**：
```bash
git status -s              # 看清范围
git diff --stat HEAD       # 看清各文件改动量
# 按主题分组：
#   1) main.go / router.go / config 相关
#   2) handlers/<resource> 相关
#   3) openapi / proto 相关
#   4) cli / docs 相关
# 每组单独 commit，主题前缀按现有规范（feat/refactor/docs）
```

---

## P1 —— 跑通业务后稳态前必收敛（按价值排序）

### P1-1 跑通完整业务剧本，归档为运行证据
**证据**：A.4 段——业务剧本与代码对齐度高，但缺真实运行回放。
**做法**（用 P0-B 引入的 RequestID 串日志）：
1. 启动 `docker-compose up -d`；
2. 跑端到端剧本：创建项目 → Room → 委派任务给 codex/claude/gemini → 上下文越界触发 handoff → 提交评审 → 自动归档；
3. 导出 trace + room messages + memory artifacts → `.arc/audit/aitask/e2e-baseline/`；
4. 把 RequestID 串起来的日志摘录写成 1 页 markdown 作为「业务可观察」第一份证据。

### P1-2 `tasks.go` 状态机单测（最大风险面）
**证据**：`backend/internal/http/handlers/tasks.go` 571 行 0 测试（E-50）。
**做法**：用表驱动测试覆盖关键状态迁移（pending → running → submitted → reviewed → completed / failed / cancelled），目标分支覆盖 ≥ 60%。

### P1-3 handlers / worker / context / identity 单测补到 60% 行覆盖
**证据**：E-50。
**做法**：优先级 worker > handlers > context > identity。worker 周期任务里的边界（超时熔断、空集合、shutdown 抢占）最容易藏脏数据 bug。

### P1-4 后端 Go test/build CI 流水线
**证据**：E-55，仅有 API 契约 CI。
**Quick Fix**：新增 `.github/workflows/backend-ci.yml`：`go vet + go test -race ./... + golangci-lint + govulncheck`，PR 必跑。

### P1-5 SCA / 依赖许可扫描
**Quick Fix**：
```bash
cd backend && govulncheck ./... > ../.arc/audit/aitask/govulncheck.txt 2>&1
cd ../fronted && pnpm audit --prod --json > ../.arc/audit/aitask/pnpm-audit.json
osv-scanner -r .  > .arc/audit/aitask/osv.txt 2>&1
```
将输出回填到 `dependency-health.md` 与 `license-risk-analysis.md`。

### P1-6 第一个版本 tag + 简短 CHANGELOG
跑通业务剧本后立刻打 tag（例如 `v0.1.0-internal`），作为内网部署可回滚基线。CHANGELOG 只需 1 段：本版本能跑通什么、配置最小集是什么、已知限制。

---

## P2 —— 演进 / 长尾（不影响内网单机使用）

| ID | 项 | 说明 |
|---|---|---|
| P2-1 | 大 handler 拆分 | `tasks.go` 571 行可抽 dto / mapper / use-case |
| P2-2 | 前端 Vitest 覆盖率提升 | features/* 关键交互补单测 |
| P2-3 | DB 慢 SQL 监控 | `pg_stat_statements` 收集（可选） |
| P2-4 | 触发器 + Worker 失败重试可观测性 | 当前靠日志，可考虑结构化失败计数 |

---

## 一键交接命令

P0 三项推荐分支落地（建议先 P0-C 收口工作树再开新分支）：
```bash
# Step 1：先收口 in-flight
git checkout -b chore/inflight-cleanup
# 按 P0-C 分组提交

# Step 2：开 P0-A / P0-B
git checkout -b feat/internal-hardening
arc:build "执行 .arc/audit/aitask/recommendations.md 中的 P0-A 与 P0-B：config 拒绝默认密钥 + RequestID/访问日志中间件"
```

P1 跑通业务后再做：
```bash
arc:e2e   "用 RequestID 串日志跑通完整业务剧本，归档到 .arc/audit/aitask/e2e-baseline/"
arc:build "为 backend/internal/http/handlers/tasks.go 状态机补单测，目标分支覆盖 ≥ 60%"
```
