import { CornerDownRight, MessageSquareWarning, Pin, User } from 'lucide-react'
import i18n from '@/i18n'
import type { MessageType, RoomMessage } from '@/api/types'
import { formatTime } from '@/lib/format'
import { AgentAvatar } from '@/components/shared/agent-avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { TaskStatusPill } from '@/components/shared/task-status-pill'
import { UlidChip } from '@/components/shared/ulid-chip'
import { cn } from '@/lib/utils'
import { SystemEventPill } from './system-event-pill'

const FALLBACK_MESSAGE_TYPES: MessageType[] = [
  'blocker',
  'review_request',
  'review_result',
  'artifact_reference',
  'memory_note',
  'context_handoff',
  'command_request',
]

const TYPE_LABEL: Partial<Record<MessageType, string>> = {
  blocker: '阻塞汇报',
  review_request: '请求审查',
  review_result: '审查结果',
  artifact_reference: '产物引用',
  memory_note: 'OpenViking 备忘',
  context_handoff: '上下文交接',
  command_request: '命令请求',
}

interface MessageBubbleProps {
  message: RoomMessage
  searchTerm?: string
  onPin?(messageId: string): void
}

export function MessageBubble({ message, searchTerm = '', onPin }: MessageBubbleProps) {
  if (message.sender.type === 'system' || message.messageType === 'system_event') {
    return <SystemEventPill message={message} />
  }

  const isOperator = message.sender.type === 'operator'
  const displayContent = normalizeMessageContent(message.content)

  return (
    <div
      className={cn(
        'flex max-w-3xl items-start gap-4',
        isOperator ? 'ml-auto flex-row-reverse' : '',
      )}
    >
      {isOperator ? (
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-emerald-200 bg-emerald-50 text-emerald-600">
          <User className="h-4 w-4" />
        </div>
      ) : (
        <AgentAvatar agentType={message.sender.agentType ?? 'system'} />
      )}

      <div className={cn('flex max-w-full flex-col', isOperator ? 'items-end' : 'items-start')}>
        <div className="mb-1 flex items-baseline gap-2 text-xs">
          <span className="font-semibold text-[hsl(var(--foreground))]">
            {senderLabel(message)}
          </span>
          <span className="text-[hsl(var(--muted-foreground))]">
            {formatTime(message.createdAt)}
          </span>
          {onPin ? (
            <Button
              variant="ghost"
              size="sm"
              className="h-5 px-1.5 text-[10px]"
              onClick={() => onPin(message.messageId)}
            >
              <Pin className="h-3 w-3" />
              pin
            </Button>
          ) : null}
        </div>

        <div
          className={cn(
            'rounded-2xl border px-4 py-3 text-sm leading-relaxed shadow-sm',
            isOperator
              ? 'rounded-tr-sm border-emerald-600 bg-emerald-600 text-white'
              : 'rounded-tl-sm border-[hsl(var(--border))] bg-[hsl(var(--card))] text-[hsl(var(--foreground))]',
          )}
        >
          <MessageHeader message={message} />
          <p className="whitespace-pre-wrap break-all">
            <HighlightedText text={displayContent} term={searchTerm} />
          </p>
          <MessagePayload message={message} isOperator={isOperator} />
        </div>
      </div>
    </div>
  )
}

function normalizeMessageContent(content: string): string {
  if (!content.includes('\n') && content.includes('\\n')) {
    return content.replaceAll('\\n', '\n')
  }
  return content
}

function HighlightedText({ text, term }: { text: string; term: string }) {
  const needle = term.trim()
  if (!needle) return text
  const lower = text.toLowerCase()
  const lowerNeedle = needle.toLowerCase()
  const parts: React.ReactNode[] = []
  let cursor = 0
  let index = lower.indexOf(lowerNeedle)
  while (index >= 0) {
    if (index > cursor) parts.push(text.slice(cursor, index))
    parts.push(
      <mark key={`${index}-${needle}`} className="rounded bg-amber-200 px-0.5 text-slate-950">
        {text.slice(index, index + needle.length)}
      </mark>,
    )
    cursor = index + needle.length
    index = lower.indexOf(lowerNeedle, cursor)
  }
  if (cursor < text.length) parts.push(text.slice(cursor))
  return <>{parts}</>
}

function senderLabel(message: RoomMessage): string {
  if (message.sender.type === 'operator') {
    return message.sender.operatorLabel ?? 'operator'
  }
  if (message.sender.type === 'agent' && message.sender.agentType) {
    return i18n.t(`agent.type.${message.sender.agentType}`)
  }
  return 'system'
}

function MessageHeader({ message }: { message: RoomMessage }) {
  if (message.messageType === 'text') return null
  if (message.messageType === 'question') {
    return (
      <Badge tone="warning" className="mb-2 inline-flex w-fit items-center gap-1">
        <MessageSquareWarning className="h-3 w-3" /> 待回应问题
      </Badge>
    )
  }
  if (message.messageType === 'answer') {
    return (
      <Badge tone="info" className="mb-2 inline-flex w-fit items-center gap-1">
        <CornerDownRight className="h-3 w-3" /> 回应
      </Badge>
    )
  }
  if (FALLBACK_MESSAGE_TYPES.includes(message.messageType)) {
    return (
      <Badge
        tone="muted"
        className="mb-2 inline-flex w-fit items-center gap-1 font-mono text-[10px]"
      >
        [{TYPE_LABEL[message.messageType] ?? message.messageType}] 完整渲染待 FE-051
      </Badge>
    )
  }
  return null
}

function MessagePayload({ message, isOperator }: { message: RoomMessage; isOperator: boolean }) {
  const payload = message.payload ?? {}
  if (message.messageType === 'task_status') {
    const taskId = typeof payload.taskId === 'string' ? payload.taskId : undefined
    const status = typeof payload.status === 'string' ? payload.status : undefined
    return (
      <div className={cn('mt-3 flex flex-wrap gap-2', isOperator ? 'justify-end' : '')}>
        {taskId ? <UlidChip id={taskId} /> : null}
        {status ? (
          <TaskStatusPill status={status as Parameters<typeof TaskStatusPill>[0]['status']} />
        ) : null}
      </div>
    )
  }
  if (message.messageType === 'task_reference') {
    const taskId = typeof payload.taskId === 'string' ? payload.taskId : undefined
    if (!taskId) return null
    return (
      <div className={cn('mt-3 flex', isOperator ? 'justify-end' : '')}>
        <UlidChip id={taskId} />
      </div>
    )
  }
  if (message.messageType === 'context_handoff') {
    const handoffId = typeof payload.handoffId === 'string' ? payload.handoffId : undefined
    if (!handoffId) return null
    return (
      <div className={cn('mt-3 flex items-center gap-2 text-xs', isOperator ? 'justify-end' : '')}>
        <span className={isOperator ? 'text-emerald-100' : 'text-[hsl(var(--muted-foreground))]'}>
          handoff
        </span>
        <UlidChip id={handoffId} />
      </div>
    )
  }
  return null
}
