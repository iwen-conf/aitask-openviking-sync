import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, ExternalLink, FileText, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { TaskStatusPill } from '@/components/shared/task-status-pill'
import { UlidChip } from '@/components/shared/ulid-chip'
import { formatRelativeTime, formatTime } from '@/lib/format'
import { Markdown } from '@/components/shared/markdown'
import { useArtifactsQuery } from '@/api/artifacts'
import {
  useCancelTaskMutation,
  useFailTaskMutation,
  useReviewTaskMutation,
  useTaskDetailQuery,
  useTaskEventsQuery,
  useTasksQuery,
} from '@/api/tasks'
import { useToast } from '@/components/ui/use-toast'
import { ArtifactPreviewDialog } from '@/features/artifacts/artifact-preview-dialog'
import { ArtifactTypeBadge } from '@/features/artifacts/artifact-type-badge'
import { ActiveRunBadge } from '@/features/tasks/active-run-badge'
import { DependencyGraph } from '@/features/tasks/dependency-graph'
import { SubtaskTree } from '@/features/tasks/subtask-tree'
import type { ArtifactSummary, Task, TaskEvent } from '@/api/types'
import { useProjectOutletContext } from './use-project-context'

export function TaskDetailRoute() {
  const { projectId } = useProjectOutletContext()
  const { taskId = '' } = useParams<{ taskId: string }>()
  const { t } = useTranslation()
  const { toast } = useToast()
  const navigate = useNavigate()
  const [reviewDialog, setReviewDialog] = useState<'approve' | 'reject' | null>(null)
  const [failDialogOpen, setFailDialogOpen] = useState(false)
  const [reason, setReason] = useState('')
  const [previewArtifactId, setPreviewArtifactId] = useState<string | undefined>()

  const detailQuery = useTaskDetailQuery(projectId, taskId || undefined)
  const eventsQuery = useTaskEventsQuery(projectId, taskId || undefined)
  const artifactsQuery = useArtifactsQuery(projectId, taskId ? { taskId } : {})
  const tasksQuery = useTasksQuery(projectId)
  const reviewTask = useReviewTaskMutation(projectId)
  const failTask = useFailTaskMutation(projectId)
  const cancelTask = useCancelTaskMutation(projectId)

  const task = detailQuery.data
  const allTasks = useMemo(() => tasksQuery.data?.items ?? [], [tasksQuery.data])

  const dependencyTasks = useMemo(() => {
    if (!task?.dependencies) return []
    return task.dependencies.map(
      (id) => allTasks.find((t) => t.taskId === id) ?? ({ taskId: id } as Task),
    )
  }, [task, allTasks])

  const goBack = () => navigate(`/projects/${projectId}/tasks`)
  const selectTask = (id: string) => navigate(`/projects/${projectId}/tasks/${id}`)

  if (!task && detailQuery.isLoading) {
    return (
      <PageShell onBack={goBack}>
        <div className="flex h-64 items-center justify-center text-[hsl(var(--muted-foreground))]">
          <Loader2 className="mr-2 h-4 w-4 animate-spin" /> {t('task.detail.loading')}
        </div>
      </PageShell>
    )
  }
  if (!task) {
    return (
      <PageShell onBack={goBack}>
        <div className="flex h-64 flex-col items-center justify-center gap-2 text-[hsl(var(--muted-foreground))]">
          <p className="text-sm">{t('task.detail.notFound')}</p>
        </div>
      </PageShell>
    )
  }

  const cancellable = !['done', 'cancelled', 'failed'].includes(task.status)
  const reviewable = ['submitted', 'reviewing'].includes(task.status)
  const failable = !['done', 'cancelled', 'failed'].includes(task.status)

  const handleReview = async () => {
    if (!reviewDialog) return
    const trimmed = reason.trim()
    if (reviewDialog === 'reject' && !trimmed) {
      toast({ title: 'Reject 必须填写原因', tone: 'destructive' })
      return
    }
    try {
      await reviewTask.mutateAsync({
        taskId: task.taskId,
        input: { approve: reviewDialog === 'approve', reason: trimmed || 'approved by operator' },
      })
      toast({
        title: reviewDialog === 'approve' ? '任务已通过 review' : '任务已驳回',
        tone: reviewDialog === 'approve' ? 'success' : 'info',
      })
      setReviewDialog(null)
      setReason('')
    } catch (error) {
      toast({
        title: 'Review 操作失败',
        description: error instanceof Error ? error.message : undefined,
        tone: 'destructive',
      })
    }
  }

  const handleFail = async () => {
    const trimmed = reason.trim()
    if (!trimmed) {
      toast({ title: '标记 Fail 必须填写原因', tone: 'destructive' })
      return
    }
    try {
      await failTask.mutateAsync({
        taskId: task.taskId,
        input: { runId: task.activeRunId ?? undefined, reason: trimmed },
      })
      toast({ title: '任务已标记为 failed', tone: 'info' })
      setFailDialogOpen(false)
      setReason('')
    } catch (error) {
      toast({
        title: '标记 Fail 失败',
        description: error instanceof Error ? error.message : undefined,
        tone: 'destructive',
      })
    }
  }

  const handleCancel = async () => {
    try {
      await cancelTask.mutateAsync(task.taskId)
      toast({ title: `任务 ${task.taskId} 已取消`, tone: 'info' })
      goBack()
    } catch (error) {
      toast({
        title: '取消失败',
        description: error instanceof Error ? error.message : undefined,
        tone: 'destructive',
      })
    }
  }

  return (
    <PageShell onBack={goBack}>
      <header className="space-y-3 border-b border-[hsl(var(--border))] pb-4">
        <div className="flex flex-wrap items-center gap-2">
          <UlidChip id={task.taskId} />
          <TaskStatusPill status={task.status} />
          <ActiveRunBadge task={task} />
        </div>
        <h1 className="pr-8 text-xl font-semibold leading-snug text-[hsl(var(--foreground))]">
          {task.title}
        </h1>
        {task.goal ? (
          <p className="text-sm font-medium text-[hsl(var(--foreground))]">
            🎯 {task.goal}
          </p>
        ) : null}
      </header>

      <div className="space-y-6 pt-6">
        <Section title="任务结构">
          <StructuredFields task={task} />
        </Section>

        <Section title="基本信息">
          <DefList
            items={[
              [
                '受托 Agent',
                task.assigneeAgentType
                  ? t(`agent.type.${task.assigneeAgentType}`)
                  : t('task.notDelegated'),
              ],
              ['Agent ID', task.assigneeAgentId ? <UlidChip id={task.assigneeAgentId} /> : '—'],
              [
                '所需技能',
                task.requiredSkills?.length ? (
                  <div className="flex flex-wrap gap-1">
                    {task.requiredSkills.map((skill) => (
                      <Badge key={skill} tone="outline">
                        {skill}
                      </Badge>
                    ))}
                  </div>
                ) : (
                  '—'
                ),
              ],
              ['指定模型', task.requiredModel ?? '—'],
              ['优先级', task.priority !== undefined ? String(task.priority) : '—'],
            ]}
          />
        </Section>

        <Section title="委托历史">
          <DefList
            items={[
              [
                '委托方',
                task.delegatedByType === 'operator'
                  ? `Operator · ${task.delegatedByOperatorLabel ?? 'local-operator'}`
                  : task.delegatedByType === 'agent'
                    ? `Agent · ${task.delegatedByAgentId ?? '—'}`
                    : 'System',
              ],
              [
                '委托时间',
                task.delegatedAt
                  ? `${formatRelativeTime(task.delegatedAt)} (${formatTime(task.delegatedAt)})`
                  : '—',
              ],
              ['创建时间', formatTime(task.createdAt)],
              ['更新时间', `${formatRelativeTime(task.updatedAt)} (${formatTime(task.updatedAt)})`],
            ]}
          />
        </Section>

        <Section title="执行状态">
          {task.status === 'running' && task.activeRunId ? (
            <DefList
              items={[
                ['Run ID', <UlidChip key="run" id={task.activeRunId} />],
                [
                  '最近心跳',
                  task.lastHeartbeatAt
                    ? `${formatRelativeTime(task.lastHeartbeatAt)} (${formatTime(task.lastHeartbeatAt)})`
                    : '—',
                ],
              ]}
            />
          ) : (
            <p className="text-sm text-[hsl(var(--muted-foreground))]">
              当前未在执行（{t(`task.status.${task.status}`)}）。
            </p>
          )}
        </Section>

        <Section title="输入上下文">
          <div className="space-y-3 text-sm">
            <div className="flex flex-wrap items-baseline gap-2">
              <span className="w-20 text-xs text-[hsl(var(--muted-foreground))]">父任务</span>
              {task.parentTaskId ? (
                <button
                  type="button"
                  onClick={() => selectTask(task.parentTaskId!)}
                  className="inline-flex items-center gap-1 rounded-md border border-indigo-200 bg-indigo-50/60 px-2 py-0.5 font-mono text-xs text-indigo-700 hover:bg-indigo-100"
                >
                  {task.parentTaskId.slice(-10)}
                  <ExternalLink className="h-3 w-3" />
                </button>
              ) : (
                <span className="text-[hsl(var(--muted-foreground))]">—</span>
              )}
            </div>
            <div className="flex flex-wrap items-baseline gap-2">
              <span className="w-20 text-xs text-[hsl(var(--muted-foreground))]">依赖任务</span>
              {dependencyTasks.length === 0 ? (
                <span className="text-[hsl(var(--muted-foreground))]">—</span>
              ) : (
                <div className="flex flex-1 flex-wrap gap-1">
                  {dependencyTasks.map((dep) => (
                    <button
                      key={dep.taskId}
                      type="button"
                      onClick={() => selectTask(dep.taskId)}
                      className="inline-flex items-center gap-1 rounded-md border border-slate-200 bg-slate-50 px-2 py-0.5 font-mono text-xs text-slate-600 hover:border-indigo-200 hover:bg-indigo-50 hover:text-indigo-700"
                    >
                      {dep.taskId.slice(-10)}
                      {dep.title ? (
                        <span className="font-sans text-[hsl(var(--muted-foreground))]">
                          · {dep.title.slice(0, 24)}
                          {dep.title.length > 24 ? '…' : ''}
                        </span>
                      ) : null}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
        </Section>

        <Section title="依赖关系图">
          <DependencyGraph current={task} allTasks={allTasks} onSelectTask={selectTask} />
        </Section>

        <Section title="子任务">
          <SubtaskTree rootTaskId={task.taskId} allTasks={allTasks} onSelectTask={selectTask} />
        </Section>

        <Section title="验收标准">
          {task.outputContract ? (
            <Markdown>{task.outputContract}</Markdown>
          ) : (
            <p className="text-sm text-[hsl(var(--muted-foreground))]">
              尚未声明验收标准。
            </p>
          )}
        </Section>

        <Section title="执行输出">
          <TaskOutputPreview
            artifacts={artifactsQuery.data?.items ?? []}
            isLoading={artifactsQuery.isLoading}
            onPreview={setPreviewArtifactId}
          />
        </Section>

        <Section title="事件历史">
          <TaskEventTimeline
            events={eventsQuery.data?.items ?? []}
            isLoading={eventsQuery.isLoading}
          />
        </Section>
      </div>

      <footer className="sticky bottom-0 z-10 -mx-6 mt-6 flex flex-wrap justify-end gap-2 border-t border-[hsl(var(--border))] bg-[hsl(var(--card))] px-6 py-3">
        <Button
          variant="outline"
          disabled={!reviewable || reviewTask.isPending}
          onClick={() => {
            setReason('')
            setReviewDialog('approve')
          }}
        >
          Review 通过
        </Button>
        <Button
          variant="ghost"
          disabled={!reviewable || reviewTask.isPending}
          onClick={() => {
            setReason('')
            setReviewDialog('reject')
          }}
        >
          Review 驳回
        </Button>
        <Button
          variant="ghost"
          disabled={!failable || failTask.isPending}
          onClick={() => {
            setReason('')
            setFailDialogOpen(true)
          }}
        >
          标记 Fail
        </Button>
        <Button
          variant="destructive"
          disabled={!cancellable || cancelTask.isPending}
          onClick={handleCancel}
        >
          取消任务
        </Button>
      </footer>

      <Dialog open={reviewDialog !== null} onOpenChange={(open) => !open && setReviewDialog(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{reviewDialog === 'approve' ? 'Review 通过' : 'Review 驳回'}</DialogTitle>
            <DialogDescription>
              {reviewDialog === 'approve'
                ? '确认后任务将进入完成流转。'
                : 'Reject 必须填写可执行的修改原因。'}
            </DialogDescription>
          </DialogHeader>
          <Textarea
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder={reviewDialog === 'approve' ? '补充 comment（可选）' : '填写 reject reason'}
          />
          <DialogFooter>
            <Button variant="ghost" onClick={() => setReviewDialog(null)}>
              取消
            </Button>
            <Button onClick={handleReview} disabled={reviewTask.isPending}>
              {reviewTask.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              确认
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={failDialogOpen} onOpenChange={setFailDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>标记任务失败</DialogTitle>
            <DialogDescription>失败原因会写入 task event，供后续恢复或重试判断。</DialogDescription>
          </DialogHeader>
          <Textarea
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder="填写失败原因"
          />
          <DialogFooter>
            <Button variant="ghost" onClick={() => setFailDialogOpen(false)}>
              取消
            </Button>
            <Button variant="destructive" onClick={handleFail} disabled={failTask.isPending}>
              {failTask.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              标记 Fail
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ArtifactPreviewDialog
        projectId={projectId}
        artifactId={previewArtifactId}
        onClose={() => setPreviewArtifactId(undefined)}
      />
    </PageShell>
  )
}

function PageShell({ children, onBack }: { children: React.ReactNode; onBack(): void }) {
  const { t } = useTranslation()
  return (
    <div className="flex h-full flex-col overflow-hidden bg-[hsl(var(--muted)/0.4)]">
      <div className="shrink-0 border-b border-[hsl(var(--border))] bg-[hsl(var(--card))] px-6 py-3">
        <Button variant="ghost" size="sm" onClick={onBack} className="gap-1">
          <ArrowLeft className="h-4 w-4" /> {t('task.detail.back')}
        </Button>
      </div>
      <div className="flex-1 overflow-y-auto">
        <div className="mx-auto max-w-4xl px-6 py-6">{children}</div>
      </div>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="space-y-2">
      <h3 className="text-xs font-semibold uppercase tracking-wider text-[hsl(var(--muted-foreground))]">
        {title}
      </h3>
      {children}
    </section>
  )
}

function DefList({ items }: { items: Array<[label: string, value: React.ReactNode]> }) {
  return (
    <dl className="grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
      {items.map(([label, value]) => (
        <div
          key={label}
          className="flex flex-col gap-0.5 rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--card))] px-3 py-2"
        >
          <dt className="text-[11px] uppercase tracking-wide text-[hsl(var(--muted-foreground))]">
            {label}
          </dt>
          <dd className="text-sm text-[hsl(var(--foreground))]">{value}</dd>
        </div>
      ))}
    </dl>
  )
}

// StructuredFields renders the 6-field delegation standard: goal/context/
// inputs/constraints are surfaced even when blank (showing "—") so the
// receiver can tell at a glance that the delegator left them out, while
// title and acceptance are surfaced in their own header / section already.
function StructuredFields({ task }: { task: Task }) {
  const { t } = useTranslation()
  const rows: Array<{ label: string; value?: string; hint?: string }> = [
    { label: t('task.field.goal.label'), value: task.goal, hint: t('task.field.goal.hint') },
    {
      label: t('task.field.context.label'),
      value: task.description,
      hint: t('task.field.context.hint'),
    },
    { label: t('task.field.inputs.label'), value: task.inputs, hint: t('task.field.inputs.hint') },
    {
      label: t('task.field.constraints.label'),
      value: task.constraints,
      hint: t('task.field.constraints.hint'),
    },
  ]

  return (
    <div className="space-y-3">
      {rows.map((row) => (
        <div
          key={row.label}
          className="rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--card))] px-3 py-2"
        >
          <div className="flex items-baseline justify-between gap-3">
            <span className="text-[11px] font-semibold uppercase tracking-wide text-[hsl(var(--muted-foreground))]">
              {row.label}
            </span>
            {row.hint ? (
              <span className="text-[10px] text-[hsl(var(--muted-foreground))]/80">{row.hint}</span>
            ) : null}
          </div>
          <div className="mt-1 text-sm text-[hsl(var(--foreground))]">
            {row.value && row.value.trim() ? (
              <Markdown>{row.value}</Markdown>
            ) : (
              <span className="text-[hsl(var(--muted-foreground))]">—</span>
            )}
          </div>
        </div>
      ))}
    </div>
  )
}

function TaskEventTimeline({ events, isLoading }: { events: TaskEvent[]; isLoading: boolean }) {
  if (isLoading && events.length === 0) {
    return (
      <p className="rounded-md border border-[hsl(var(--border))] p-3 text-sm text-[hsl(var(--muted-foreground))]">
        正在加载事件历史…
      </p>
    )
  }
  if (events.length === 0) {
    return (
      <p className="rounded-md border border-dashed border-[hsl(var(--border))] p-3 text-sm text-[hsl(var(--muted-foreground))]">
        暂无 task event。
      </p>
    )
  }
  return (
    <ol className="space-y-3">
      {events.map((event) => (
        <li key={event.eventId} className="flex gap-3">
          <span className="mt-1 h-2 w-2 shrink-0 rounded-full bg-indigo-500" />
          <div className="min-w-0 flex-1 rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--card))] p-3">
            <div className="flex flex-wrap items-center gap-2 text-xs">
              <Badge tone="outline">{event.eventType}</Badge>
              {event.fromStatus ? <TaskStatusPill status={event.fromStatus} /> : null}
              {event.toStatus ? (
                <>
                  <span className="text-[hsl(var(--muted-foreground))]">→</span>
                  <TaskStatusPill status={event.toStatus} />
                </>
              ) : null}
              <span className="ml-auto text-[hsl(var(--muted-foreground))]">
                {formatRelativeTime(event.createdAt)}
              </span>
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-[hsl(var(--muted-foreground))]">
              <span>actor: {event.actorType}</span>
              {event.actorAgentId ? <UlidChip id={event.actorAgentId} /> : null}
              {event.actorOperatorLabel ? <span>{event.actorOperatorLabel}</span> : null}
            </div>
          </div>
        </li>
      ))}
    </ol>
  )
}

function TaskOutputPreview({
  artifacts,
  isLoading,
  onPreview,
}: {
  artifacts: ArtifactSummary[]
  isLoading: boolean
  onPreview(artifactId: string): void
}) {
  if (isLoading && artifacts.length === 0) {
    return (
      <p className="rounded-md border border-[hsl(var(--border))] p-3 text-sm text-[hsl(var(--muted-foreground))]">
        正在加载 artifacts…
      </p>
    )
  }
  if (artifacts.length === 0) {
    return (
      <p className="rounded-md border border-dashed border-[hsl(var(--border))] p-3 text-sm text-[hsl(var(--muted-foreground))]">
        暂无输出产物；任务 submit 后会在这里展示 result 和 artifacts。
      </p>
    )
  }
  return (
    <div className="space-y-2">
      {artifacts.slice(0, 6).map((artifact) => (
        <button
          key={artifact.artifactId}
          type="button"
          onClick={() => onPreview(artifact.artifactId)}
          className="flex w-full items-center gap-3 rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--card))] p-3 text-left transition-colors hover:border-indigo-200 hover:bg-indigo-50/50"
        >
          <FileText className="h-4 w-4 text-indigo-500" />
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-medium">{artifact.name}</span>
            <span className="block truncate font-mono text-[11px] text-[hsl(var(--muted-foreground))]">
              {artifact.path}
            </span>
          </span>
          <ArtifactTypeBadge type={artifact.artifactType} />
        </button>
      ))}
    </div>
  )
}
