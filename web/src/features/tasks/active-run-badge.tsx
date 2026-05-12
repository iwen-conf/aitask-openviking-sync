import { useEffect, useState } from 'react'
import { Activity, AlertTriangle } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { Task } from '@/api/types'

const HEARTBEAT_TIMEOUT_MS = 90_000
const CLOCK_SKEW_TOLERANCE_MS = 5_000
const TICK_MS = 5_000

interface ActiveRunBadgeProps {
  task: Task
}

export function ActiveRunBadge({ task }: ActiveRunBadgeProps) {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (task.status !== 'running' || !task.activeRunId) return
    const id = setInterval(() => setNow(Date.now()), TICK_MS)
    return () => clearInterval(id)
  }, [task.status, task.activeRunId])

  if (task.status !== 'running' || !task.activeRunId) return null

  const lastBeatAt = task.lastHeartbeatAt ?? task.updatedAt
  const lastBeat = Date.parse(lastBeatAt)
  if (Number.isNaN(lastBeat)) return null

  const elapsed = Math.max(0, now - lastBeat - CLOCK_SKEW_TOLERANCE_MS)
  const overdue = elapsed >= HEARTBEAT_TIMEOUT_MS

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-semibold',
        overdue
          ? 'bg-rose-50 text-rose-700 ring-1 ring-rose-200'
          : 'bg-emerald-50 text-emerald-700 ring-1 ring-emerald-200',
      )}
      title={`最近心跳：${new Date(lastBeat).toLocaleTimeString()}`}
    >
      {overdue ? (
        <>
          <AlertTriangle className="h-3 w-3" />
          心跳超时 {Math.round(elapsed / 1000)}s
        </>
      ) : (
        <>
          <Activity className="h-3 w-3 animate-pulse" />
          运行中
        </>
      )}
    </span>
  )
}
