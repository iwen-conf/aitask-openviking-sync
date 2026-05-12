# Evidence Registry — aitask 内网单机就绪度审计

审计基线：`main` @ `afb7770`，本地工作树存在 55 个未提交文件改动（+2268/-641），审计仅基于已提交基线 + 关键未提交差异作为参考。

> **部署假设**（信息源：`docs/API/README.md` §强约束）：无用户管理 / 内网受信 / docker-compose 单机部署 / 不上 Kubernetes。
> 下文所有「事实」条目均不变；新增「内网单机定位下风险等级」一栏，区分原审计（按公网假设）与当前定位下的真实风险。

## 1. 仓库结构与构建

| ID | 证据 | 路径 / 来源 |
|---|---|---|
| E-1 | Go 1.25 + 多二进制（server/openviking/healthcheck 等） | `backend/go.mod:3`, `backend/cmd/{server,openviking,healthcheck,aitask,review-perf-helper}` |
| E-2 | 后端依赖：gin v1.11、pgx v5.9、redis v9.19、gorilla/websocket v1.5、ulid v2、connectrpc | `backend/go.mod:6-12` |
| E-3 | 前端：React 19 + Vite + Tailwind + Radix + Zustand + TanStack Query + Zod + RHF + Framer Motion | `fronted/package.json:21-52` |
| E-4 | 前端 dev 依赖：playwright、vitest、testing-library、eslint、prettier | `fronted/package.json:54-72` |
| E-5 | 多阶段构建 + distroless nonroot 运行时 | `backend/Dockerfile:1-25` |
| E-6 | 前端 nginx:1.27-alpine 运行时 + envsubst 注入 + 健康检查 + 长缓存 | `fronted/Dockerfile:1-44`, `fronted/nginx.conf:1-58` |
| E-7 | docker-compose 含 postgres + dragonfly + backend + web + openviking，全部带 healthcheck 与 restart | `docker-compose.yml:1-110` |

## 2. 配置与运行时

| ID | 证据 | 路径 | 内网单机定位下风险 |
|---|---|---|---|
| E-10 | 配置加载 + 全字段 Validate 但**仅校验非空**，未拒绝默认值 | `backend/internal/config/config.go:140-289`, `:405-410` | 🟡 即使内网，默认值进生产环境变量也是操作风险（P0-A） |
| E-11 | `defaultAgentSecret = "dev_only_change_me"` 作为内置默认 | `backend/internal/config/config.go:23` | 🟡 P0-A |
| E-12 | `defaultPostgresPass = "aitask_dev_password"` / `defaultDragonflyPass = "dragonfly_dev_password"` | `backend/internal/config/config.go:18,21` | 🟡 P0-A |
| E-13 | `IsProduction()` 仅用于 `gin.SetMode(ReleaseMode)`，未用于安全约束 | `backend/internal/config/config.go:296-298`, `backend/cmd/server/main.go:42-44` | 🟡 P0-A 拟将 `IsProduction()` 联动默认密钥拒绝 |
| E-14 | DATABASE_URL 默认 `sslmode=disable` | `backend/internal/config/config.go:215-222`, `.env.example` | ✅ 内网容器互联可接受，演进项 |
| E-15 | docker-compose 中 backend 直接以 `dev_only_change_me` 等默认值兜底 | `docker-compose.yml:62-63` | 🟡 P0-A 配套：兜底改 `${VAR:?must set}` |
| E-16 | `.env.example` 含 `CORS_ALLOWED_ORIGINS=http://localhost:3000` | `.env.example:24` | ✅ 内网同源部署不需要，将从 `.env.example` 移除以免误导 |

## 3. 安全 / 鉴权 / 限流

| ID | 证据 | 路径 | 内网单机定位下风险 |
|---|---|---|---|
| E-20 | 无 `Authorization` 头时**默认按 operator 处理放行**，无 prod 强制开关 | `backend/internal/http/middleware/auth.go:31-37` | ✅ **设计意图，非风险**（`docs/API/README.md` 第 7、11 行） |
| E-21 | Bearer 校验失败返回 401（AGENT_TOKEN_INVALID/EXPIRED） | `backend/internal/http/middleware/auth.go:40-62` | — |
| E-22 | `RequireProjectAccess` 仅作用于 agent 身份，operator 直接放行 | `backend/internal/http/middleware/auth.go:67-86` | ✅ **设计意图，非风险**（Web Console 操作者非权限主体） |
| E-23 | 令牌桶限流（Redis Lua 原子 HMSET + PEXPIRE） | `backend/internal/http/middleware/ratelimit.go:1-50` | ✅ 内网仍有价值，防 Worker / Agent 风暴 |
| E-24 | 全局未注册 CORS / RequestID / 访问日志 中间件，仅 `gin.Recovery()` | `backend/internal/http/router.go:35-37` | 🟡 CORS 不需要（同源部署）；**RequestID + 访问日志仍要补**（P0-B），是排障可见性 |
| E-25 | 路由层：项目级路径强制 `RequireProjectAccess` | `backend/internal/http/router.go:64-110` | — |
| E-26 | 21 个标准化错误码 + 单测 | `backend/internal/http/codes/codes.go`, `codes_test.go` | — |
| E-27 | 错误响应 envelope `{code,message,retriable,details}` | `backend/internal/http/handlers/errors.go:1-23` | — |
| E-28 | distroless 镜像 + USER nonroot | `backend/Dockerfile:21-23` | — |

## 4. 数据 / 迁移 / 持久层

| ID | 证据 | 路径 |
|---|---|---|
| E-30 | 12 个 SQL migration（带 up/down），覆盖 projects/sessions/agents/tasks/artifacts/room/handoffs/snapshots | `migrations/postgres/000001..000012` |
| E-31 | pgx v5 + database/sql 接入 | `backend/cmd/server/main.go:50-55` |
| E-32 | startup 时执行 readiness（含 postgres/dragonfly critical 检查），失败 exit 1 | `backend/cmd/server/main.go:71-86` |

## 5. 健康 / 可观测 / 调度

| ID | 证据 | 路径 |
|---|---|---|
| E-40 | `/healthz` + `/readyz` HTTP 探针 | `backend/internal/http/router.go:39-40` |
| E-41 | 容器层 healthcheck：`/app/healthcheck` 子命令 | `backend/Dockerfile:14-18`, `docker-compose.yml:73-78` |
| E-42 | 结构化日志 `log/slog` 在 service / worker / rpc 多处使用 | `backend/cmd/server/main.go:38`, `internal/service/**/service.go` |
| E-43 | Worker scheduler 多周期任务（active_run_timeout / progress / presence / completion / handoff / cron daily summary），全部含 graceful shutdown | `backend/cmd/server/main.go:300-389` |
| E-44 | 优雅关闭：SIGINT/SIGTERM + ctx + Shutdown Timeout | `backend/cmd/server/main.go:395-429` |
| E-45 | 未发现 `/metrics`、prometheus、OpenTelemetry、tracing | grep 全仓无命中 |

## 6. 测试 / CI

| ID | 证据 | 路径 |
|---|---|---|
| E-50 | 后端单测：21 个 `*_test.go`，覆盖 config / cli / http / codes / middleware / agents / health / openviking / projects / room / sessions / tasks / pkg/ids；handlers / worker / context / identity / rpc 无测试 | `find backend -name '*_test.go'` |
| E-51 | `go test ./...` 全部 PASS（缓存命中） | 本地执行 |
| E-52 | 前端单测：仅 2 个 `.test.*` / `.spec.*` 文件（约 95 个 src TS/TSX） | `find fronted/src` |
| E-53 | E2E：Playwright，2 个 e2e 脚本 | `fronted/e2e/`, `fronted/playwright.config.ts` |
| E-54 | CI：`backend-api-contracts.yml` (buf + 契约同步 + 产物上传) + `frontend-ci.yml` (lint→test→build→e2e + 产物上传) | `.github/workflows/` |
| E-55 | CI 仅在 `paths` 命中或 `main` 推送时触发，不存在 backend Go 单测/构建专门流水线 | `.github/workflows/backend-api-contracts.yml:3-15` |

## 7. 文档 / 契约 / API

| ID | 证据 | 路径 |
|---|---|---|
| E-60 | `docs/API/` 含 7 份接口契约 (`projects/tasks/agents/context/room/memory-artifacts/health-errors`) | `docs/API/*.md` |
| E-61 | `api/openapi/openapi.yaml` + `api/protobuf/aitask/v1` + `api/websocket/agent-room-envelope.schema.json` 多协议契约 | `api/` |
| E-62 | 看板风格任务文档：前端/后端/审查 三轨 × 5 状态 | `docs/{前端,后端,审查}/tasks/*.md` |
| E-63 | 顶层规格 `ai_agent_project_orchestrator_requirements_v2.md` 单一信源 | `docs/` |

## 8. 工作树状态

| ID | 证据 | 路径 |
|---|---|---|
| E-70 | 55 个未提交修改文件，涵盖 main.go / router / handlers / openapi / cli 等核心区域，+2268/-641 | `git status` / `git diff --stat` |
| E-71 | 最近 20 次提交主题以 `feat/refactor/docs` 为主，无 release tag | `git log` |
| E-72 | 未发现版本/CHANGELOG | `find` 无 `CHANGELOG*` |
