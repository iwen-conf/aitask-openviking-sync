# License Risk Analysis — aitask

> **状态**：本次审计未运行 SBOM/SCA 工具（要求 read-only 且未授权安装额外工具），结论基于 `go.mod` / `package.json` 公开许可证常识。
> 真实生产前**必须**用 `pnpm licenses list` / `go-licenses` / `osv-scanner` 自动化复核。

## Backend (Go) — `backend/go.mod`

| 依赖 | 已知许可证 | 风险等级 | 备注 |
|---|---|---|---|
| github.com/gin-gonic/gin | MIT | 低 | 商业可用 |
| github.com/jackc/pgx/v5 | MIT | 低 | 商业可用 |
| github.com/redis/go-redis/v9 | BSD-2 | 低 | 商业可用 |
| github.com/gorilla/websocket | BSD-2 | 低 | 商业可用 |
| github.com/oklog/ulid/v2 | Apache-2.0 | 低 | 商业可用 |
| github.com/DATA-DOG/go-sqlmock | BSD-3 | 低 | 仅测试依赖 |
| connectrpc.com/connect | Apache-2.0 | 低 | 商业可用 |
| github.com/bytedance/sonic | Apache-2.0 | 低 | 商业可用 |
| github.com/spf13/cobra / pflag | Apache-2.0 / BSD-3 | 低 | 商业可用 |
| github.com/quic-go/quic-go | MIT | 低 | 商业可用 |
| go.uber.org/* | MIT | 低 | 商业可用 |
| golang.org/x/* | BSD-3 | 低 | 商业可用 |
| google.golang.org/protobuf | BSD-3 | 低 | 商业可用 |

**初步结论**：Go 侧未观察到 GPL/AGPL/CC-BY-NC/SSPL 等具有传染性或禁止商用的许可证。**待 `go-licenses csv ./... ` 复核全部传递依赖**。

## Frontend (npm) — `fronted/package.json`

| 依赖 | 已知许可证 | 风险等级 | 备注 |
|---|---|---|---|
| react / react-dom | MIT | 低 | |
| @tanstack/react-query | MIT | 低 | |
| @radix-ui/* | MIT | 低 | |
| zustand | MIT | 低 | |
| zod | MIT | 低 | |
| react-hook-form | MIT | 低 | |
| react-router-dom | MIT | 低 | |
| framer-motion | MIT | 低 | |
| tailwindcss | MIT | 低 | |
| lucide-react | ISC | 低 | |
| i18next / react-i18next | MIT | 低 | |
| @dnd-kit/core | MIT | 低 | |
| class-variance-authority | Apache-2.0 | 低 | |
| eslint / prettier / typescript | MIT / Apache-2.0 | 低 | dev only |
| @playwright/test | Apache-2.0 | 低 | dev only |

**初步结论**：未观察到 GPL/AGPL/SSPL/Commons-Clause/CC-BY-NC 等限制项。但前端版本号声明非常激进（React 19.2.5、TS 6.0.2、Vite 8、Vitest 4、ESLint 10、Tailwind 4），存在两类潜在风险：
1. 实际安装版本可能落到不同主版本，传递依赖许可可能漂移。
2. 部分新版可能尚未稳定，影响生产可靠性（许可虽不变，但工程风险联动）。

## 必做动作（合规闭环）

```bash
# Go 侧
go install github.com/google/go-licenses@latest
go-licenses csv ./... > .arc/audit/aitask/go-licenses.csv

# 前端侧
cd fronted && pnpm licenses list --prod > ../.arc/audit/aitask/pnpm-licenses.txt

# 漏洞 & 许可双重视角
osv-scanner -r . > .arc/audit/aitask/osv.txt
```

将三份输出回填本文件，明确每条**禁止商用 / 强 copyleft** 依赖的处置（替换 / 隔离 / 法务豁免）。
