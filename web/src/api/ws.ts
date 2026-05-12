import type { WsConnectionStatus, WsEnvelope } from './types'

export interface RoomSocketHandlers {
  onEnvelope(envelope: WsEnvelope): void
  onStatus(status: WsConnectionStatus): void
}

export interface RoomSocketHandle {
  close(): void
}

const PING_INTERVAL_MS = 25_000
const MAX_BACKOFF_MS = 30_000

/**
 * 真实 WebSocket 客户端：自动重连（指数退避，封顶 30 s），25 s ping 心跳。
 * 收到的 envelope 与 §10.3 schema 一致；调用方负责把事件落到 React Query cache 或 store。
 */
export function createRoomSocket(url: string, handlers: RoomSocketHandlers): RoomSocketHandle {
  let socket: WebSocket | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let pingTimer: ReturnType<typeof setInterval> | null = null
  let attempt = 0
  let manuallyClosed = false

  const cleanupSocket = () => {
    if (pingTimer) {
      clearInterval(pingTimer)
      pingTimer = null
    }
    if (socket) {
      socket.onopen = null
      socket.onmessage = null
      socket.onclose = null
      socket.onerror = null
    }
  }

  const scheduleReconnect = () => {
    if (manuallyClosed) return
    handlers.onStatus('reconnecting')
    attempt += 1
    const delay = Math.min(MAX_BACKOFF_MS, 1_000 * 2 ** Math.min(attempt, 5))
    reconnectTimer = setTimeout(connect, delay)
  }

  const connect = () => {
    if (manuallyClosed) return
    handlers.onStatus(attempt === 0 ? 'connecting' : 'reconnecting')

    try {
      socket = new WebSocket(url)
    } catch (error) {
      console.error('[room-socket] failed to create WebSocket', error)
      scheduleReconnect()
      return
    }

    socket.onopen = () => {
      attempt = 0
      handlers.onStatus('open')
      pingTimer = setInterval(() => {
        if (socket?.readyState === WebSocket.OPEN) {
          socket.send(JSON.stringify({ type: 'ping', sentAt: new Date().toISOString() }))
        }
      }, PING_INTERVAL_MS)
    }

    socket.onmessage = (event) => {
      if (typeof event.data !== 'string') return
      try {
        const data = JSON.parse(event.data) as WsEnvelope
        if (data && typeof data === 'object' && 'eventType' in data) {
          handlers.onEnvelope(data)
        }
      } catch (error) {
        console.warn('[room-socket] received non-JSON payload', error)
      }
    }

    socket.onclose = () => {
      cleanupSocket()
      if (manuallyClosed) {
        handlers.onStatus('closed')
        return
      }
      scheduleReconnect()
    }

    socket.onerror = () => {
      socket?.close()
    }
  }

  connect()

  return {
    close() {
      manuallyClosed = true
      if (reconnectTimer) {
        clearTimeout(reconnectTimer)
        reconnectTimer = null
      }
      cleanupSocket()
      socket?.close()
      handlers.onStatus('closed')
    },
  }
}
