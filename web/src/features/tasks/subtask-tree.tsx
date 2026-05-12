import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Bot, ChevronDown, ChevronRight } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { TASK_STATUS_GROUP } from '@/lib/format'
import type { Task } from '@/api/types'
import { cn } from '@/lib/utils'

interface SubtaskTreeProps {
  rootTaskId: string
  allTasks: Task[]
  onSelectTask(taskId: string): void
}

interface ChildIndex {
  byParent: Map<string, Task[]>
}

function buildIndex(tasks: Task[]): ChildIndex {
  const byParent = new Map<string, Task[]>()
  for (const t of tasks) {
    if (!t.parentTaskId) continue
    const arr = byParent.get(t.parentTaskId)
    if (arr) arr.push(t)
    else byParent.set(t.parentTaskId, [t])
  }
  for (const arr of byParent.values()) {
    arr.sort((a, b) => a.createdAt.localeCompare(b.createdAt))
  }
  return { byParent }
}

function countDescendants(taskId: string, index: ChildIndex): number {
  const direct = index.byParent.get(taskId) ?? []
  let total = direct.length
  for (const child of direct) total += countDescendants(child.taskId, index)
  return total
}

export function SubtaskTree({ rootTaskId, allTasks, onSelectTask }: SubtaskTreeProps) {
  const { t } = useTranslation()
  const index = useMemo(() => buildIndex(allTasks), [allTasks])
  const directChildren = index.byParent.get(rootTaskId) ?? []

  if (directChildren.length === 0) {
    return <p className="text-sm text-[hsl(var(--muted-foreground))]">{t('task.subtree.empty')}</p>
  }

  return (
    <ul className="space-y-1">
      {directChildren.map((child) => (
        <SubtaskNode
          key={child.taskId}
          task={child}
          depth={0}
          index={index}
          onSelectTask={onSelectTask}
        />
      ))}
    </ul>
  )
}

interface SubtaskNodeProps {
  task: Task
  depth: number
  index: ChildIndex
  onSelectTask(taskId: string): void
}

function SubtaskNode({ task, depth, index, onSelectTask }: SubtaskNodeProps) {
  const { t } = useTranslation()
  const directChildren = index.byParent.get(task.taskId) ?? []
  const hasChildren = directChildren.length > 0
  const [open, setOpen] = useState(depth < 1)

  const descendantCount = useMemo(
    () => (hasChildren ? countDescendants(task.taskId, index) : 0),
    [task.taskId, index, hasChildren],
  )

  const group = TASK_STATUS_GROUP[task.status]
  const tone =
    group === 'blocked'
      ? 'warning'
      : group === 'done'
        ? 'muted'
        : group === 'running'
          ? 'success'
          : 'muted'

  return (
    <li>
      <div
        className={cn(
          'group flex items-center gap-1 rounded-md border border-transparent py-1 pr-2 hover:border-[hsl(var(--border))] hover:bg-[hsl(var(--muted)/0.5)]',
        )}
        style={{ paddingLeft: `${depth * 16 + 4}px` }}
      >
        {hasChildren ? (
          <button
            type="button"
            onClick={() => setOpen((v) => !v)}
            className="flex h-5 w-5 shrink-0 items-center justify-center rounded text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--muted))]"
            aria-label={open ? t('task.subtree.collapse') : t('task.subtree.expand')}
          >
            {open ? (
              <ChevronDown className="h-3.5 w-3.5" />
            ) : (
              <ChevronRight className="h-3.5 w-3.5" />
            )}
          </button>
        ) : (
          <span className="h-5 w-5 shrink-0" />
        )}
        <button
          type="button"
          onClick={() => onSelectTask(task.taskId)}
          className="flex flex-1 items-center gap-2 truncate text-left text-sm"
        >
          <Bot className="h-3.5 w-3.5 text-[hsl(var(--muted-foreground))]" />
          <span className="flex-1 truncate text-[hsl(var(--foreground))]">{task.title}</span>
          {hasChildren ? (
            <span className="shrink-0 font-mono text-[10px] text-[hsl(var(--muted-foreground))]">
              {t('task.subtree.countSuffix', { count: descendantCount })}
            </span>
          ) : null}
          <Badge tone={tone} className="shrink-0">
            {t(`task.status.${task.status}`)}
          </Badge>
        </button>
      </div>
      {hasChildren && open ? (
        <ul className="mt-0.5 space-y-0.5">
          {directChildren.map((child) => (
            <SubtaskNode
              key={child.taskId}
              task={child}
              depth={depth + 1}
              index={index}
              onSelectTask={onSelectTask}
            />
          ))}
        </ul>
      ) : null}
    </li>
  )
}
