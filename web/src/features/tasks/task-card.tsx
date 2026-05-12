import { motion } from 'framer-motion'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { GripVertical, MoreVertical } from 'lucide-react'
import { useDraggable } from '@dnd-kit/core'
import { CSS } from '@dnd-kit/utilities'
import {
  TASK_STATUS_DOT,
  TASK_STATUS_GROUP,
  TASK_STATUS_PULSE,
  formatRelativeTime,
} from '@/lib/format'
import { Card } from '@/components/ui/card'
import { useAgentsQuery } from '@/api/agents'
import { cardEnter } from '@/components/shared/motion-presets'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import type { Agent, AgentType, Task } from '@/api/types'
import { isDraggableTask } from './draggable'

interface TaskCardProps {
  task: Task
  onCancel(taskId: string): void
  onSelect(taskId: string): void
  /** 用于 DragOverlay 渲染时静态展示,不挂 dnd-kit listeners */
  staticPreview?: boolean
}

export function TaskCard({ task, onCancel, onSelect, staticPreview = false }: TaskCardProps) {
  const { t } = useTranslation()
  const draggable = isDraggableTask(task) && !staticPreview
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: task.taskId,
    data: { task },
    disabled: !draggable,
  })

  const group = TASK_STATUS_GROUP[task.status]
  const isFinal = group === 'done'
  const agentsQuery = useAgentsQuery()
  const delegator = useDelegatorLabel(task, agentsQuery.data?.items, t)

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      onSelect(task.taskId)
    }
  }

  const style = transform ? { transform: CSS.Translate.toString(transform) } : undefined
  const statusLabel = t(`task.status.${task.status}`)

  return (
    <motion.div
      ref={setNodeRef}
      variants={cardEnter}
      layout
      style={style}
      className={isDragging ? 'opacity-30' : ''}
    >
      <Card
        role="button"
        tabIndex={0}
        onClick={() => !staticPreview && onSelect(task.taskId)}
        onKeyDown={handleKeyDown}
        aria-label={`${task.title} · ${statusLabel}`}
        className={cn(
          'p-4 transition-shadow hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[hsl(var(--ring))] focus-visible:ring-offset-2',
          staticPreview ? 'cursor-grabbing shadow-xl' : 'cursor-pointer',
          isFinal ? 'opacity-80' : '',
          group === 'blocked' ? 'border-orange-200 bg-orange-50/40 hover:border-orange-300' : '',
        )}
      >
        <div className="flex items-start gap-2">
          {draggable ? (
            <button
              type="button"
              aria-label={t('task.card.dragHandle')}
              className="-ml-1 mt-0.5 flex h-5 w-4 shrink-0 cursor-grab items-center justify-center text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))] active:cursor-grabbing"
              {...listeners}
              {...attributes}
              onClick={(e) => e.stopPropagation()}
            >
              <GripVertical className="h-3.5 w-3.5" />
            </button>
          ) : null}

          <span
            aria-label={statusLabel}
            title={statusLabel}
            className={cn(
              'mt-1.5 inline-block h-2.5 w-2.5 shrink-0 rounded-full ring-2 ring-white',
              TASK_STATUS_DOT[task.status],
              TASK_STATUS_PULSE[task.status] ? 'animate-pulse' : '',
            )}
          />

          <h4
            className={cn(
              'min-w-0 flex-1 truncate text-sm font-semibold leading-snug',
              isFinal
                ? 'text-[hsl(var(--muted-foreground))] line-through'
                : 'text-[hsl(var(--foreground))]',
              group === 'blocked' ? 'text-orange-800' : '',
            )}
          >
            {task.title}
          </h4>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                onClick={(e) => e.stopPropagation()}
                className="-mr-1 -mt-1 shrink-0 rounded-md p-1 text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--muted))] hover:text-[hsl(var(--foreground))]"
              >
                <MoreVertical className="h-4 w-4" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={() => onSelect(task.taskId)}>
                {t('task.card.view')}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onCancel(task.taskId)}>
                {t('task.card.cancel')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        {task.description ? (
          <p
            className="mt-2 truncate text-xs text-[hsl(var(--muted-foreground))]"
            title={task.description}
          >
            {task.description}
          </p>
        ) : null}

        <div className="mt-3 flex items-center justify-between gap-2 text-[11px] text-[hsl(var(--muted-foreground))]">
          <span className="inline-flex min-w-0 items-center gap-1">
            <span>{t('task.card.delegator')}:</span>
            <span className="truncate font-medium text-[hsl(var(--foreground))]">{delegator}</span>
          </span>
          <span className="shrink-0">{formatRelativeTime(task.updatedAt)}</span>
        </div>

        {task.status === 'reviewing' || task.status === 'submitted' ? (
          <p className="mt-2 text-[11px] text-violet-600">
            {t('task.card.reviewingHint', { status: statusLabel })}
          </p>
        ) : null}
      </Card>
    </motion.div>
  )
}

/**
 * 委托者展示：
 * - operator → operatorLabel（不展示原始 ULID）
 * - agent    → 已知 agent 类型映射为「Claude Code / Codex / Gemini」，否则回落 ULID 末段
 * - system   → i18n「系统」
 */
function useDelegatorLabel(task: Task, agents: Agent[] | undefined, t: TFunction): string {
  if (task.delegatedByType === 'operator') {
    return task.delegatedByOperatorLabel ?? 'operator'
  }
  if (task.delegatedByType === 'agent') {
    const agentId = task.delegatedByAgentId
    if (!agentId) return t('task.card.delegatorSystem')
    const matched = agents?.find((a) => a.agentId === agentId)
    if (matched) {
      const type = matched.agentType as AgentType
      return t(`agent.type.${type}`)
    }
    return agentId.includes('_') ? agentId.split('_').slice(-1)[0] : agentId
  }
  return t('task.card.delegatorSystem')
}
