# AgentFlow Web Console

`docs/前端/` 任务清单的 React 实现，覆盖 Projects 列表 / Tasks 看板 / Agent 协作室三大主页面。

## 技术栈

| 层   | 选型                                      | 用途                                                        |
| ---- | ----------------------------------------- | ----------------------------------------------------------- |
| 构建 | Vite + TypeScript（严格模式）             | HMR + 类型检查                                              |
| UI   | Tailwind CSS v4 + 手写 shadcn/ui 风格组件 | 与设计稿一致的视觉层                                        |
| 状态 | Zustand + TanStack Query                  | UI 状态 + 服务器同步                                        |
| 路由 | React Router v7                           | `/projects`、`/projects/:projectId/{overview,tasks,room,…}` |
| 表单 | React Hook Form + Zod                     | 创建项目 / 创建任务 / 发送消息                              |
| 动效 | Framer Motion                             | 列表入场、卡片切换                                          |
| 图标 | lucide-react                              | 单源图标库                                                  |

依赖与 `docs/前端/README.md §1` 完全对齐；不引入登录/会话相关库。

## 启动前提

前端**没有 mock 层**，所有数据来自真实后端。开发与运行前必须先起后端 REST / WebSocket 网关（参见 `docs/后端/`）。

推荐先从 `web/.env.example` 复制为 `web/.env.local`，再按下面两种开发模式选一种：

1. 同源代理模式（推荐，适合同机开发）
   - `VITE_API_BASE_URL` / `VITE_WS_BASE_URL` 留空。
   - `VITE_DEV_PROXY_TARGET` 指向后端，例如 `http://127.0.0.1:8080`。
   - 前端请求 `/api` 和 `/ws` 会由 Vite dev server 代理到后端。
2. 直连后端模式
   - 显式填写后端地址，例如：
     ```dotenv
     VITE_API_BASE_URL=http://127.0.0.1:8080
     VITE_WS_BASE_URL=ws://127.0.0.1:8080
     ```

> 后端未就绪时，所有列表/详情会落到错误状态（`describeError` 转中文），WebSocket 状态显示为「未连接」。

## 启动

```bash
pnpm install
pnpm dev          # 默认监听 5173
pnpm build        # tsc -b && vite build
pnpm lint
```

## 部署方式

- 开发环境：`pnpm dev` + `vite.config.ts` 的 `server.proxy`（仅 dev 生效）。
- 生产环境：使用 `web/Dockerfile` 构建镜像（`node:22-alpine` build + `nginx:alpine` runtime）。
- 生产时通过 `runtime-config.template.js` + `docker-entrypoint.d/40-runtime-config.sh` 在容器启动注入 `VITE_API_BASE_URL` / `VITE_WS_BASE_URL`，无需重新构建镜像。
- `vite proxy` 不参与生产流量。

## 目录结构

```
src/
├── api/                     docs/API 镜像 + TanStack Query hooks + WebSocket 客户端
│   ├── client.ts            fetch 封装 + 错误码映射
│   ├── ws.ts                WebSocket 客户端：自动重连 + 心跳
│   ├── types.ts             docs/API DTO
│   ├── errors.ts            21 个错误码 → 中文文案
│   ├── projects.ts          列表 / 详情 / 创建 / 更新 / 完成 / 归档
│   ├── tasks.ts             看板查询 + 创建 / 取消 / fail / review
│   ├── agents.ts            列表 + 撤销 Token
│   ├── memory.ts            目录 / 读 / 搜索 / 写入
│   ├── artifacts.ts         列表 + 详情
│   └── room.ts              Room / 消息 / 发送 / pin / unread mentions / WebSocket
├── lib/                     工具函数（cn / 时间格式化 / 状态映射 / Markdown 渲染 / useOperatorLabel）
│   └── markdown.ts          内置 Markdown 渲染器（XSS 安全；heading / 列表 / 代码 / 表格 / 链接）
├── stores/                  Zustand：toast 队列 / WS 连接状态
├── components/
│   ├── ui/                  手写精简版 shadcn/ui 基础组件
│   ├── layout/              AppShell / Sidebar / Topbar / 项目状态徽章
│   └── shared/              空态 / 错误边界 / ULID 徽章 / Agent 头像 / 任务状态 pill / Motion 预设
├── features/
│   ├── projects/            列表 + 创建对话框
│   ├── tasks/               看板（按 Agent 分列，列内 §21.3 三段映射）+ 创建对话框
│   ├── room/                Agent 协作室 + 消息渲染（含 6 种 message_type 实化 + 7 种 fallback）
│   ├── memory/              OpenViking 浏览：树形导航 / Markdown Viewer / 搜索 / 写入对话框
│   ├── artifacts/           Artifacts 列表卡片 + diff 渲染器 + 类型徽章 + 预览对话框
│   └── agents/              Agent 列表 / 详情 / Token 撤销对话框（二次确认）
├── routes/                  React Router 路由表（含 Memory / Artifacts / Agents / Settings 完整实现）
└── App.tsx / main.tsx       入口（QueryClient + Router + Toaster + ErrorBoundary）
```

## 与项目契约的关系

- 所有 DTO 定义在 `src/api/types.ts`，字段命名遵循 `docs/API/`。docs 改动需先动 docs 再同步该文件。
- 错误码到中文文案的映射在 `src/api/errors.ts`，覆盖 §22 列出的 21 个错误码。
- 看板列布局采用「按 Agent 分列」，列内分组保留 §21.3 三段语义。详见 `docs/前端/decisions.md` D-FE-001。
- Room 当前实化 6 种 + 1 fallback，对其余 7 种 message_type 留出占位标签。详见 `docs/前端/decisions.md` D-FE-002。
- Memory Browser 渲染器为内置极简 Markdown（lib/markdown.ts），全文 escapeHtml；mermaid / 数学公式后续增强。
- 不存在 `/login` `/register` 等路由；Topbar 的 operatorLabel 来自后端项目详情接口（`useOperatorLabel`）。

## WebSocket 行为

`src/api/ws.ts` 实现：

- 连接地址：`${VITE_WS_BASE_URL}/ws/projects/:projectId/agent-room`
- 当 `VITE_WS_BASE_URL` 为空时，浏览器端自动回退到当前前端 origin（`ws://` 或 `wss://`），可直接配合同源代理/反代模式。
- 重连策略：指数退避，封顶 30 s。
- 心跳：每 25 s 发送 `{ type: "ping", sentAt }`，由后端按 §10 envelope 协议处理。
- 收到的 envelope 由 `useRoomSocket` 转成 React Query invalidate：
  - `room.message` / `context.handoff_created` → 刷新消息列表
  - `task.updated` → 刷新任务看板
  - `room.member_online` / `room.member_offline` / `room.connected` → 刷新成员列表

## 后续接入点

| 区域               | 文件                                   | 后续工作                                                          |
| ------------------ | -------------------------------------- | ----------------------------------------------------------------- |
| 完整 message_type  | `src/features/room/message-bubble.tsx` | 实现 fallback 列表中的 7 种渲染（FE-051 子任务）。                |
| 全局 operatorLabel | `src/lib/use-operator-label.ts`        | 当前从项目详情派生；如后续新增全局端点，需先更新 `docs/API`。     |
| Markdown 增强      | `src/lib/markdown.ts`                  | mermaid / 代码语法高亮 / 数学公式（$KaTeX$）按需扩展。            |
| 任务详情抽屉       | `src/features/tasks/`                  | FE-033 ~ FE-037：Task 详情、事件时间线、依赖图、review 决策面板。 |
