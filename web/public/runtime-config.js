// 运行时配置占位。本地开发由 vite 直接服务该文件，字段为空字符串，
// `src/lib/env.ts` 自动回退到 `import.meta.env` (即 `.env.local`)。
// 生产镜像启动时由 `docker-entrypoint.d/40-runtime-config.sh` 通过 envsubst
// 用 VITE_API_BASE_URL / VITE_WS_BASE_URL 覆盖此文件，无需重新构建。
window.__RUNTIME_CONFIG__ = {
  VITE_API_BASE_URL: '',
  VITE_WS_BASE_URL: '',
}
