/**
 * 运行时环境变量。
 *
 * 优先级：`window.__RUNTIME_CONFIG__` (容器启动时由 envsubst 注入) > `import.meta.env`
 * (本地 `.env.local`)。前者使生产镜像可在不重新构建的前提下切换 base URL；后者负责本地默认值。
 * 若未显式配置 `VITE_WS_BASE_URL`，浏览器端自动回退为当前 origin 对应的 ws/wss 地址，
 * 既可配合 Vite proxy，也可配合同源 Nginx `/ws` 反代。
 */
declare global {
  interface Window {
    __RUNTIME_CONFIG__?: {
      VITE_API_BASE_URL?: string
      VITE_WS_BASE_URL?: string
    }
  }
}

type RuntimeKey = 'VITE_API_BASE_URL' | 'VITE_WS_BASE_URL'

function readRuntime(key: RuntimeKey): string {
  if (typeof window !== 'undefined') {
    const raw = window.__RUNTIME_CONFIG__?.[key]
    // envsubst 未替换时保留 `${VAR}` 字面量，按未配置处理。
    if (raw && !raw.startsWith('${')) {
      const trimmed = raw.trim()
      if (trimmed) return trimmed
    }
  }
  return ((import.meta.env[key] as string | undefined) ?? '').trim()
}

function fallbackBrowserWsBaseUrl(): string {
  if (typeof window === 'undefined') return ''
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}`
}

const wsBaseUrl = readRuntime('VITE_WS_BASE_URL') || fallbackBrowserWsBaseUrl()

export const env = {
  apiBaseUrl: readRuntime('VITE_API_BASE_URL'),
  wsBaseUrl,
} as const
