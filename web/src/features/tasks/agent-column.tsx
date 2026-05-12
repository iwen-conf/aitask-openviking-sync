import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Bot } from 'lucide-react'
import { useDroppable } from '@dnd-kit/core'
import { TASK_STATUS_GROUP } from '@/lib/format'
import { AgentAvatar } from '@/components/shared/agent-avatar'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { Agent, Task } from '@/api/types'
import { StatusSection } from './status-section'

interface AgentColumnProps {
  agent: Agent
  tasks: Task[]
  onCancel(taskId: string): void
  onSelect(taskId: string): void
}

export function AgentColumn({ agent, tasks, onCancel, onSelect }: AgentColumnProps) {
  const { t } = useTranslation()
  const { setNodeRef, isOver, active } = useDroppable({
    id: `column:${agent.agentType}`,
    data: { agent },
  })

  const sourceAgentType = (active?.data?.current as { task?: Task } | undefined)?.task
    ?.assigneeAgentType
  const willAccept = active && sourceAgentType !== agent.agentType

  const grouped = useMemo(() => {
    const buckets = {
      queue: [] as Task[],
      running: [] as Task[],
      blocked: [] as Task[],
      done: [] as Task[],
    }
    for (const task of tasks) {
      buckets[TASK_STATUS_GROUP[task.status]].push(task)
    }
    return buckets
  }, [tasks])

  return (
    <section
      ref={setNodeRef}
      className={cn(
        'flex h-full min-w-0 flex-1 flex-col rounded-xl border bg-[hsl(var(--card)/0.65)] transition-colors',
        isOver && willAccept
          ? 'border-indigo-400 bg-indigo-50/40 ring-2 ring-indigo-300'
          : 'border-[hsl(var(--border))]',
      )}
    >
      <header className="flex items-center justify-between gap-2 rounded-t-xl border-b border-[hsl(var(--border))] bg-[hsl(var(--card))] p-4">
        <div className="flex items-center gap-3 overflow-hidden">
          <AgentAvatar agentType={agent.agentType} />
          <div className="overflow-hidden">
            <div className="truncate text-sm font-semibold text-[hsl(var(--foreground))]">
              {t(`agent.type.${agent.agentType}`)}
            </div>
            <div className="truncate text-xs text-[hsl(var(--muted-foreground))]">
              {t(`agent.role.${agent.agentType}`)} · {agent.name}
            </div>
          </div>
        </div>
        <Badge tone="outline" className="font-mono">
          {t('task.counter', { count: tasks.length })}
        </Badge>
      </header>

      <div className="flex-1 space-y-4 overflow-y-auto scrollbar-thin p-3">
        <StatusSection
          groupKey="running"
          tasks={grouped.running}
          onCancel={onCancel}
          onSelect={onSelect}
        />
        <StatusSection
          groupKey="blocked"
          tasks={grouped.blocked}
          onCancel={onCancel}
          onSelect={onSelect}
        />
        <StatusSection
          groupKey="queue"
          tasks={grouped.queue}
          onCancel={onCancel}
          onSelect={onSelect}
        />
        <StatusSection
          groupKey="done"
          tasks={grouped.done}
          onCancel={onCancel}
          onSelect={onSelect}
        />

        {tasks.length === 0 ? (
          <div className="flex h-32 flex-col items-center justify-center gap-2 text-[hsl(var(--muted-foreground))]">
            <Bot className="h-6 w-6 opacity-50" />
            <p className="text-xs">{t('task.emptyColumn')}</p>
          </div>
        ) : null}
      </div>
    </section>
  )
}
