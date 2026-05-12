import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ArrowRight, ListChecks, MessagesSquare } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { useProjectQuery } from '@/api/projects'
import { useTasksQuery } from '@/api/tasks'
import { useRoomMessagesQuery } from '@/api/room'
import { TASK_STATUS_GROUP, formatRelativeTime } from '@/lib/format'
import { TaskStatusPill } from '@/components/shared/task-status-pill'
import { useProjectOutletContext } from './use-project-context'

export function OverviewRoute() {
  const { t } = useTranslation()
  const { projectId } = useProjectOutletContext()
  const projectQuery = useProjectQuery(projectId)
  const tasksQuery = useTasksQuery(projectId)
  const messagesQuery = useRoomMessagesQuery(projectId)

  const tasks = tasksQuery.data?.items ?? []
  const messages = messagesQuery.data?.items ?? []

  const stats = {
    total: tasks.length,
    queue: tasks.filter((task) => TASK_STATUS_GROUP[task.status] === 'queue').length,
    running: tasks.filter((task) => TASK_STATUS_GROUP[task.status] === 'running').length,
    blocked: tasks.filter((task) => TASK_STATUS_GROUP[task.status] === 'blocked').length,
    done: tasks.filter((task) => TASK_STATUS_GROUP[task.status] === 'done').length,
  }

  const recentTasks = [...tasks].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)).slice(0, 5)

  const recentMessages = [...messages].slice(-5).reverse()

  return (
    <div className="grid h-full grid-cols-1 gap-4 overflow-y-auto bg-[hsl(var(--muted)/0.4)] p-6 lg:grid-cols-3">
      <Card className="lg:col-span-1">
        <CardContent className="space-y-3 p-5">
          <h3 className="text-sm font-semibold text-[hsl(var(--muted-foreground))]">
            {t('overview.metrics')}
          </h3>
          <Stat label={t('overview.metricTotal')} value={stats.total} />
          <Stat label={t('task.group.running')} value={stats.running} accent="text-emerald-600" />
          <Stat label={t('task.group.blocked')} value={stats.blocked} accent="text-orange-600" />
          <Stat label={t('task.group.queue')} value={stats.queue} accent="text-amber-600" />
          <Stat label={t('task.group.done')} value={stats.done} accent="text-indigo-600" />
        </CardContent>
      </Card>

      <Card className="lg:col-span-1">
        <CardContent className="space-y-3 p-5">
          <header className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-[hsl(var(--muted-foreground))]">
              {t('overview.recentTasks')}
            </h3>
            <Link
              to="../tasks"
              className="inline-flex items-center text-xs font-medium text-[hsl(var(--primary))]"
            >
              <ListChecks className="mr-1 h-3.5 w-3.5" /> {t('overview.boardLink')}
            </Link>
          </header>
          <ul className="space-y-2">
            {recentTasks.map((task) => (
              <li
                key={task.taskId}
                className="rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--card))] p-2 text-xs text-[hsl(var(--foreground))]"
              >
                <div className="flex items-center justify-between">
                  <span className="line-clamp-1">{task.title}</span>
                  <TaskStatusPill status={task.status} />
                </div>
                <div className="mt-1 text-[11px] text-[hsl(var(--muted-foreground))]">
                  {formatRelativeTime(task.updatedAt)}
                </div>
              </li>
            ))}
            {recentTasks.length === 0 ? (
              <p className="text-xs text-[hsl(var(--muted-foreground))]">
                {t('overview.emptyTasks')}
              </p>
            ) : null}
          </ul>
        </CardContent>
      </Card>

      <Card className="lg:col-span-1">
        <CardContent className="space-y-3 p-5">
          <header className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-[hsl(var(--muted-foreground))]">
              {t('overview.recentMessages')}
            </h3>
            <Link
              to="../room"
              className="inline-flex items-center text-xs font-medium text-[hsl(var(--primary))]"
            >
              <MessagesSquare className="mr-1 h-3.5 w-3.5" /> {t('overview.roomLink')}
            </Link>
          </header>
          <ul className="space-y-2">
            {recentMessages.map((message) => (
              <li
                key={message.messageId}
                className="space-y-1 text-xs text-[hsl(var(--foreground))]"
              >
                <p className="text-[11px] text-[hsl(var(--muted-foreground))]">
                  {formatRelativeTime(message.createdAt)} · {message.sender.type}
                </p>
                <p className="line-clamp-2">{message.content}</p>
              </li>
            ))}
            {recentMessages.length === 0 ? (
              <p className="text-xs text-[hsl(var(--muted-foreground))]">
                {t('overview.emptyMessages')}
              </p>
            ) : null}
          </ul>
        </CardContent>
      </Card>

      <Card className="lg:col-span-3">
        <CardContent className="flex flex-wrap items-center justify-between gap-4 p-5 text-sm text-[hsl(var(--muted-foreground))]">
          <div>
            <p className="text-base font-semibold text-[hsl(var(--foreground))]">
              {projectQuery.data?.description || t('overview.descriptionFallback')}
            </p>
            <p className="mt-1 text-xs">
              {t('overview.policyLabel')}
              {Object.values(projectQuery.data?.completionPolicy ?? {}).join(' · ')}
            </p>
          </div>
          <Link
            to="../tasks"
            className="inline-flex items-center gap-1 rounded-md bg-[hsl(var(--primary))] px-4 py-2 text-sm font-medium text-[hsl(var(--primary-foreground))] hover:bg-[hsl(var(--primary)/0.9)]"
          >
            {t('overview.enterTasks')} <ArrowRight className="h-4 w-4" />
          </Link>
        </CardContent>
      </Card>
    </div>
  )
}

function Stat({ label, value, accent }: { label: string; value: number; accent?: string }) {
  return (
    <div className="flex items-baseline justify-between">
      <span className="text-xs text-[hsl(var(--muted-foreground))]">{label}</span>
      <span className={accent ?? 'text-[hsl(var(--foreground))]'}>
        <strong className="text-lg font-semibold">{value}</strong>
      </span>
    </div>
  )
}
