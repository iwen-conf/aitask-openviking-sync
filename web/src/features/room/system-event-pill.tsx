import { ShieldCheck } from 'lucide-react'
import type { RoomMessage } from '@/api/types'
import { formatTime } from '@/lib/format'

interface SystemEventPillProps {
  message: RoomMessage
}

/**
 * §16.3 / §10.6 强约束：系统消息样式专属，操作者与 Agent 不可复刻。
 * 使用锁形图标 + 阴影盒形 + 背景纹理来与普通消息区分。
 */
export function SystemEventPill({ message }: SystemEventPillProps) {
  return (
    <div className="flex justify-center">
      <div className="inline-flex items-center gap-2 rounded-full border border-slate-300 bg-slate-100 px-3 py-1 font-mono text-xs text-slate-600 shadow-sm">
        <ShieldCheck className="h-3 w-3 text-slate-500" />
        <span>{message.content}</span>
        <span className="text-slate-400">{formatTime(message.createdAt)}</span>
      </div>
    </div>
  )
}
