import { env } from '@/lib/env'
import { ApiError, type ApiErrorEnvelope } from './errors'

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE'
  body?: unknown
  query?: Record<string, string | number | undefined>
}

function buildUrl(path: string, query?: RequestOptions['query']): string {
  const base = env.apiBaseUrl.replace(/\/$/, '')
  const fallbackOrigin = typeof window === 'undefined' ? 'http://localhost' : window.location.origin
  const url = new URL(
    path.startsWith('/') ? `${base}${path}` : `${base}/${path}`,
    base || fallbackOrigin,
  )
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== null && value !== '') {
        url.searchParams.set(key, String(value))
      }
    }
  }
  return base ? url.toString() : url.pathname + url.search
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const method = options.method ?? 'GET'

  const response = await fetch(buildUrl(path, options.query), {
    method,
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
    },
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  })

  if (!response.ok) {
    let envelope: ApiErrorEnvelope
    try {
      envelope = (await response.json()) as ApiErrorEnvelope
    } catch {
      envelope = {
        code: 'UNKNOWN',
        message: `HTTP ${response.status}`,
        retriable: response.status >= 500,
      }
    }
    throw new ApiError(envelope)
  }

  if (response.status === 204) {
    return undefined as T
  }

  return (await response.json()) as T
}
