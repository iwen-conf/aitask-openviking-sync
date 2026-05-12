# AI Task Backend

Go backend for the AI Agent Project Orchestrator.

## 部署假设（强约束）

 > 信息源：本 README。旧 `docs/API` 目录已退役并移除。

- **不做用户管理**：无 login / register / refresh / logout / users API；Web Console 操作者仅以 `operatorLabel` 标记，不作为权限主体。
- **内网 / 单机受信部署**：通过 docker-compose 单机编排即为目标形态，**不上 Kubernetes**。
- **公网访问由外层网关负责**，本系统内不增加用户体系。
- Agent 身份只能来自 Agent Token（`Authorization: Bearer <token>`）；无 token 的请求自动视为本地 operator 并放行——这是有意设计，与上面三条一致。

## Structure

```text
core/
├── cmd/server              HTTP service entrypoint
├── internal/config         Environment-backed runtime configuration
├── internal/http           Gin router, handlers, and HTTP server construction
└── internal/service        Application services
```

The layout follows the direction of the VOMS backend, but BE-000 keeps only the minimal layers needed for a runnable service. Future tasks can add `internal/domain`, `internal/usecase`, and `internal/infrastructure` without changing the entrypoint contract.

## Commands

```bash
go test ./...
go run ./cmd/server
```

The service exposes `GET /healthz` and `GET /readyz`.

## OpenViking (Local Dev)

Current strategy is intentional: keep the in-repo file-backed adapter `aitask-openviking`
(`core/cmd/openviking`) as the default local/runtime OpenViking implementation for MVP
and runtime review scripts. Backend integrates through `OPENVIKING_BASE_URL` and remains
compatible with replacing this endpoint by an external OpenViking-compatible service later.

- Compose internal URL (backend -> openviking): `http://openviking:9090`
- Host URL (manual check): `http://127.0.0.1:${OPENVIKING_PORT:-19090}`
- Backend URL (host): `http://127.0.0.1:${APP_PORT:-8080}`

Quick start:

```bash
cp .env.example .env
bash scripts/dev-up.sh
curl -fsS "http://127.0.0.1:${APP_PORT:-8080}/readyz"
curl -fsS "http://127.0.0.1:${OPENVIKING_PORT:-19090}/healthz"
```

Expected readiness behavior:

- OpenViking unavailable => `/readyz` is `degraded` (non-critical dependency).
- Postgres/Dragonfly unavailable => `/readyz` is `not_ready`.
