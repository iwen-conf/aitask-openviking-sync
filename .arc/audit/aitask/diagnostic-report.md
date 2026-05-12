# Diagnostic Report — aitask 内网单机就绪度

**审计目标**：判定项目能否在**内网 / 单机受控环境**进入「实际跑通业务」状态。
**深度**：standard。**范围**：七维 + 依赖许可。**基线**：`main@afb7770` + 工作树未提交差异参考。
**部署假设**（取自 `docs/API/README.md` §强约束）：
- ✅ **不做用户管理**：无 login / register / refresh / logout / users API。
- ✅ **Web Console 按本地或内网受信控制台处理**；公网访问由本系统外层网关负责，不在本系统内增加用户体系。
- ✅ Agent 身份只能来自 Agent Token；Web Console 操作者只用 `operatorLabel` 标记。
- ✅ **不部署到 Kubernetes**；docker-compose 单机部署即为目标形态。

**结论一句话**：**当前工程基线已可在内网 / 单机投入使用**。原版（按公网假设打分）列为阻断的「无 Authorization 自动 operator 放行」「缺 CORS」「无 k8s/备份/回滚」「前端没注入 Authorization」等问题，**在当前部署假设下全部不再构成阻断**。剩余修补只服务于「跑通业务 + 排障可见性」，预计 2-3 天内可收敛。

---

## A. Observed Facts（仅事实，不含建议）

### A.1 架构与运行形态
- 后端 Go 1.25 单体多入口（`server` 主服务 + `openviking` 内嵌向量空间 + `healthcheck` 副命令 + `aitask` CLI），通过 `cmd/*` 拆分二进制（E-1）。
- 服务依赖 PostgreSQL 18.3 + Dragonfly v1.38.1（Redis 协议）+ openviking 自研记忆服务，以 docker-compose 编排，全部容器具备 healthcheck 与 `restart: unless-stopped`（E-7）。
- 前端为 Vite 构建的 SPA，运行时通过 nginx + envsubst 注入 `runtime-config.js`，静态资源使用 hash + `immutable` 长缓存（E-6）。
- HTTP 路由分两层：公开 `/healthz`、`/readyz`、`/ws/...`；`/api/*` 进入鉴权链；`/api/projects/:projectId/*` 进入 `RequireProjectAccess`（E-25）。
- ConnectRPC server 与 Worker 调度器在同一进程内挂载，Worker 含 7 个周期任务 + 1 个 cron 日报任务，全部带 graceful shutdown（E-43, E-44）。

### A.2 安全与鉴权（按内网无用户态口径重述）
- **设计意图（合规）**：鉴权中间件 `ResolveIdentity` 在没有 `Authorization` 头时**自动注入 operator 身份并放行**（E-20），这与 `docs/API/README.md` 声明的「Web Console 不做用户管理」一致，不构成风险。
- Bearer Token 校验失败、过期分别返回 `AGENT_TOKEN_INVALID` / `AGENT_TOKEN_EXPIRED`，由 `agents.Service` 完成（E-21）。
- `RequireProjectAccess` 仅对 agent 身份执行 ACL，operator/system 直接放行（E-22）—— **这是有意设计**：Web Console 操作者拥有全部项目可见性，agent 才走 token-scope ACL。
- 限流采用 Redis Lua 原子令牌桶，按 token/IP 维度生效（E-23）；默认容量 120，速率 2 r/s。**内网仍有价值**——可避免 Worker / Agent 风暴打满 Postgres。
- **CORS 未启用** 在内网同源部署下不构成问题（前端通过 nginx + `runtime-config.js` 拼接同源后端地址，无跨域）。
- **未注册 RequestID / 访问日志中间件**（E-24）—— **这是当前唯一仍待补的中间件**，与排障可见性强相关。
- `SecurityConfig.Validate()` 仅校验 `AGENT_TOKEN_SECRET` 非空，**未拒绝默认 `dev_only_change_me`**；DB / Dragonfly 密码同样仅校验非空（E-10, E-11, E-12）—— **即使内网部署，默认值进生产环境变量也属于操作风险**，建议启动时拒绝。
- 默认 `DATABASE_URL` 携带 `sslmode=disable`（E-14）—— 在 docker-compose 内网通信下可接受，留作后续再议。
- 镜像采用 `gcr.io/distroless/static-debian12:nonroot` + `USER nonroot`（E-28），运行时攻击面极小。

### A.3 代码质量
- 后端 83 个源文件 / 21 个测试文件，`config / http / http/codes / http/middleware / cli / rpc / service/{agents,health,openviking,projects,room,sessions,tasks} / pkg/ids` 模块均带单测；**`handlers / worker / service/context / identity` 无测试**（E-50）。
- `go test ./...` 全部 PASS（E-51）。
- 错误码集中定义为 21 项常量并有去重 + 计数测试（E-26）；HTTP 错误统一 envelope（E-27）。
- 未在 `internal/` 检索到 `TODO/FIXME/HACK` 或 `console.log/debugger`。
- `panic(` 在 `internal/` 无业务调用（仅 framework recover 与 mock 场景）。

### A.4 业务流可观察性
- 文档侧给出明确业务剧本：项目 → Room → 任务委派 → Agent 运行 → Submit/Review → Artifact → Memory 写回 → Handoff（E-60..E-63）。
- 路由侧覆盖：projects 6 路、agents 5 路 + token revoke、tasks 全状态机（list/create/get/events/patch/delegate/cancel/start/heartbeat/submit/review/fail/resume）、artifacts 2 路、room 7 路 + WebSocket、memory/skills 6 路、context/handoff 多路（router.go 全文）。
- Worker 周期对齐业务剧本：active run 超时熔断、submitted-review 兜底、project progress 刷新、presence 清理、handoff 同步、completion policy 触发系统消息、room 日报 cron。
- **缺失**：未观察到端到端的真实跑通运行报告 / 多 Agent 协同录像 / 性能基线。**这是当前最优先要补的"运行证据"**。

### A.5 DevOps / 交付（按内网单机口径重述）
- 两条 GitHub Actions：API 契约同步 + 前端 lint→test→build→e2e；**未发现后端 Go 的 build/test 流水线**（E-54, E-55），跑通业务后建议补。
- 容器化与 docker-compose 完整，**docker-compose 单机部署即为目标形态**，不需要 k8s manifest / helm chart / terraform。
- 健康/就绪探针、startup 关键依赖检查、SIGTERM 优雅关闭、Worker shutdown 超时控制均到位（E-32, E-44）。
- **不再列为缺口**：`/metrics`、tracing、错误聚合、SLO/告警、灰度/蓝绿、备份/恢复演练、回滚预案、密钥管理（vault/sops）—— 这些在内网单机受控环境下都属过度工程，留作演进项。

### A.6 团队协同
- 三轨看板（前端 / 后端 / 审查）× 5 状态（未完成 / 进行中 / 待审查 / 已完成 / 已阻塞）+ 决策日志 + 单一信源 spec，体系化程度高（E-62, E-63）。
- 提交日志主题清晰、模块前缀规范（`feat(backend)/refactor/docs/build`），20 提交内可追溯到 BE-022 → BE-139 任务编号。
- 工作树存在 55 个未提交文件改动（+2268/-641），主分支有大量 in-flight 修改未合并（E-70）。

### A.7 技术债务 / 依赖风险
- 前端核心生态版本声明（React 19.2.5、TypeScript 6.0.2、Vite 8、Vitest 4、ESLint 10、Tailwind 4.2、Framer Motion 12.38）相对激进，需要逐项核对实际锁定版本是否真实存在并稳定（见 `dependency-health.md` 备忘）。
- `tasks/todo.md` 显示项目仍有进行中的待办（git diff 显示 +19 行），尚处活跃开发阶段。
- 无 CHANGELOG / 版本 tag / release artifact，意味着没有可回滚的版本基线（E-72）—— 内网单机阶段可放，**但跑通业务后建议至少打第一个 tag 作为基线**。

---

## B. Recommendations（与上面观察一一映射，按优先级）

> 详见 `recommendations.md`。已根据「无用户态 / 无公网 / 无 k8s」前提精简：
> - **删除**原 P0-1（鉴权放行后门）—— 已是设计意图。
> - **删除**原 P0-3（CORS）—— 内网同源部署不需要。
> - **删除**原 P1-5（k8s/备份/回滚）—— 不在部署目标内。
> - **删除**原 P1-6（前端 Authorization 注入）—— 无用户态无此通道。
> - **保留并保留为**：P0-A 拒绝默认密钥 / P0-B RequestID + 访问日志 / P0-C 工作树收口。
