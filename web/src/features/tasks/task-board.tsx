import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Plus } from 'lucide-react'
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { useAgentsQuery } from '@/api/agents'
import {
  useCancelTaskMutation,
  useDelegateTaskMutation,
  useTasksQuery,
  type TaskFilters,
} from '@/api/tasks'
import { ApiError } from '@/api/errors'
import { TASK_STATUS_GROUP } from '@/lib/format'
import { useToast } from '@/components/ui/use-toast'
import type { AgentType, Task, TaskStatus } from '@/api/types'
import { AgentColumn } from './agent-column'
import { CreateTaskDialog } from './create-task-dialog'
import { isDraggableTask } from './draggable'
import { TaskCard } from './task-card'
import { TaskFilterBar } from './task-filter-bar'

const AGENT_ORDER: AgentType[] = ['claude-code', 'codex', 'gemini']
const VALID_STATUS = new Set<TaskStatus>([
  'planned',
  'delegated',
  'running',
  'submitted',
  'reviewing',
  'done',
  'blocked',
  'failed',
  'cancelled',
])
const VALID_AGENT = new Set<AgentType>(['claude-code', 'codex', 'gemini'])

function paramsToFilters(params: URLSearchParams): TaskFilters {
  const status = params.get('status')
  const assignee = params.get('assigneeAgentType')
  const skill = params.get('skill')
  const q = params.get('q')
  return {
    status: status && VALID_STATUS.has(status as TaskStatus) ? (status as TaskStatus) : undefined,
    assigneeAgentType:
      assignee && VALID_AGENT.has(assignee as AgentType) ? (assignee as AgentType) : undefined,
    skill: skill ?? undefined,
    q: q ?? undefined,
  }
}

interface TaskBoardProps {
  projectId: string
}

export function TaskBoard({ projectId }: TaskBoardProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [createOpen, setCreateOpen] = useState(false)
  const [searchParams, setSearchParams] = useSearchParams()
  const filters = useMemo(() => paramsToFilters(searchParams), [searchParams])
  const [draftQ, setDraftQ] = useState(filters.q ?? '')
  const lastSyncedQ = useRef(filters.q ?? '')
  const tasksQuery = useTasksQuery(projectId, filters)
  const agentsQuery = useAgentsQuery()
  const { toast } = useToast()
  const cancelTask = useCancelTaskMutation(projectId)
  const delegateTask = useDelegateTaskMutation(projectId)
  const [activeDragTask, setActiveDragTask] = useState<Task | null>(null)

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor),
  )

  const tasks = useMemo(() => tasksQuery.data?.items ?? [], [tasksQuery.data])
  const agents = useMemo(() => {
    const all = agentsQuery.data?.items ?? []
    const byType = new Map<AgentType, (typeof all)[number]>()
    for (const agent of all) {
      if (!byType.has(agent.agentType)) byType.set(agent.agentType, agent)
    }
    return AGENT_ORDER.map((type) => byType.get(type)).filter(
      (agent): agent is NonNullable<typeof agent> => Boolean(agent),
    )
  }, [agentsQuery.data])

  const skillOptions = useMemo(() => {
    const set = new Set<string>()
    for (const agent of agentsQuery.data?.items ?? []) {
      for (const skill of agent.skills ?? []) set.add(skill)
    }
    return Array.from(set).sort()
  }, [agentsQuery.data])

  const stats = useMemo(() => {
    const acc = { total: tasks.length, queue: 0, running: 0, blocked: 0, done: 0 }
    for (const task of tasks) {
      acc[TASK_STATUS_GROUP[task.status]] += 1
    }
    return acc
  }, [tasks])

  const handleFiltersChange = useCallback(
    (next: TaskFilters) => {
      const params = new URLSearchParams(searchParams)
      const apply = (key: string, value: string | undefined) => {
        if (value) params.set(key, value)
        else params.delete(key)
      }
      apply('status', next.status)
      apply('assigneeAgentType', next.assigneeAgentType)
      apply('skill', next.skill)
      apply('q', next.q)
      setSearchParams(params, { replace: true })
    },
    [searchParams, setSearchParams],
  )

  useEffect(() => {
    const upstream = filters.q ?? ''
    if (upstream !== lastSyncedQ.current) {
      lastSyncedQ.current = upstream
      setDraftQ(upstream)
      return
    }
    if (draftQ === upstream) return
    const id = setTimeout(() => {
      const trimmed = draftQ.trim()
      lastSyncedQ.current = trimmed
      handleFiltersChange({ ...filters, q: trimmed ? trimmed : undefined })
    }, 300)
    return () => clearTimeout(id)
  }, [draftQ, filters, handleFiltersChange])

  const handleCancel = async (taskId: string) => {
    try {
      await cancelTask.mutateAsync(taskId)
      toast({ title: `任务 ${taskId} 已取消`, tone: 'info' })
    } catch (error) {
      toast({
        title: '取消失败',
        description: error instanceof Error ? error.message : undefined,
        tone: 'destructive',
      })
    }
  }

  const handleSelectTask = useCallback(
    (taskId: string) => {
      navigate(`/projects/${projectId}/tasks/${taskId}`)
    },
    [navigate, projectId],
  )

  const handleDragStart = useCallback((event: DragStartEvent) => {
    const task = (event.active.data.current as { task?: Task } | undefined)?.task
    setActiveDragTask(task ?? null)
  }, [])

  const handleDragCancel = useCallback(() => setActiveDragTask(null), [])

  const handleDragEnd = useCallback(
    async (event: DragEndEvent) => {
      setActiveDragTask(null)
      const { active, over } = event
      if (!over) return
      const droppableId = String(over.id)
      if (!droppableId.startsWith('column:')) return
      const targetAgentType = droppableId.slice('column:'.length) as AgentType
      const task = (active.data.current as { task?: Task } | undefined)?.task
      const targetAgent = (
        over.data.current as { agent?: { agentId: string; agentType: AgentType } } | undefined
      )?.agent
      if (!task || !targetAgent) return
      if (task.assigneeAgentType === targetAgentType) return
      if (!isDraggableTask(task)) return

      try {
        await delegateTask.mutateAsync({
          taskId: task.taskId,
          agentId: targetAgent.agentId,
          agentType: targetAgentType,
          reason: '通过看板改派',
        })
        toast({
          title: `已改派到 ${targetAgentType}`,
          description: `任务 ${task.taskId.slice(-8)} 已重新委托`,
          tone: 'success',
        })
      } catch (error) {
        const message =
          error instanceof ApiError
            ? error.envelope.message
            : error instanceof Error
              ? error.message
              : '后端拒绝了改派操作'
        toast({ title: '改派失败,已回滚', description: message, tone: 'destructive' })
      }
    },
    [delegateTask, toast],
  )

  const isLoading = tasksQuery.isLoading || agentsQuery.isLoading

  return (
    <div className="flex h-full flex-1 flex-col overflow-hidden bg-[hsl(var(--muted)/0.4)]">
      <div className="z-10 shrink-0 border-b border-[hsl(var(--border))] bg-[hsl(var(--card))] p-6 shadow-sm">
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <h1 className="text-2xl font-bold tracking-tight text-[hsl(var(--foreground))]">
              执行节点看板
            </h1>
            <p className="text-sm text-[hsl(var(--muted-foreground))]">
              按受托 Agent 分列；列内沿 §21.3 三段映射分组（等待中 / 进行中 / 等待干预 / 已完成）。
            </p>
          </div>
          <Button onClick={() => setCreateOpen(true)} className="gap-2">
            <Plus className="h-4 w-4" /> 新建委托任务
          </Button>
        </div>

        <div className="mt-4 flex items-center gap-6">
          <div className="flex items-baseline gap-2">
            <span className="text-2xl font-bold text-[hsl(var(--foreground))]">{stats.total}</span>
            <span className="text-xs font-semibold uppercase tracking-wider text-[hsl(var(--muted-foreground))]">
              总任务
            </span>
          </div>
          <Separator orientation="vertical" className="h-8" />
          <KpiPill color="bg-emerald-500" label={t('task.group.running')} value={stats.running} />
          <KpiPill color="bg-orange-500" label={t('task.group.blocked')} value={stats.blocked} />
          <KpiPill color="bg-amber-400" label={t('task.group.queue')} value={stats.queue} />
          <KpiPill color="bg-slate-400" label={t('task.group.done')} value={stats.done} />
        </div>

        <div className="mt-4">
          <TaskFilterBar
            filters={filters}
            skillOptions={skillOptions}
            draftQ={draftQ}
            onDraftQChange={setDraftQ}
            onChange={handleFiltersChange}
          />
        </div>
      </div>

      <div className="flex-1 overflow-hidden p-6">
        <DndContext
          sensors={sensors}
          onDragStart={handleDragStart}
          onDragEnd={handleDragEnd}
          onDragCancel={handleDragCancel}
        >
          <div className="flex h-full items-stretch gap-6 pb-2">
            {isLoading
              ? Array.from({ length: 3 }).map((_, idx) => (
                  <div
                    key={idx}
                    className="h-full min-w-0 flex-1 animate-pulse rounded-xl border border-[hsl(var(--border))] bg-[hsl(var(--muted))]"
                  />
                ))
              : agents.map((agent) => (
                  <AgentColumn
                    key={agent.agentId}
                    agent={agent}
                    tasks={tasks.filter((task) => task.assigneeAgentType === agent.agentType)}
                    onCancel={handleCancel}
                    onSelect={handleSelectTask}
                  />
                ))}
          </div>
          <DragOverlay dropAnimation={null}>
            {activeDragTask ? (
              <div className="w-[316px]">
                <TaskCard
                  task={activeDragTask}
                  onCancel={() => undefined}
                  onSelect={() => undefined}
                  staticPreview
                />
              </div>
            ) : null}
          </DragOverlay>
        </DndContext>
      </div>

      <CreateTaskDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        projectId={projectId}
        agents={agents}
      />
    </div>
  )
}

function KpiPill({ color, label, value }: { color: string; label: string; value: number }) {
  return (
    <div className="flex items-center gap-2 text-sm text-[hsl(var(--muted-foreground))]">
      <span className={`h-2 w-2 rounded-full ${color}`} />
      <span>
        <strong className="text-[hsl(var(--foreground))]">{value}</strong> {label}
      </span>
    </div>
  )
}
