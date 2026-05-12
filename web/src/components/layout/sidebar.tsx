import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { NavLink, useLocation, useNavigate, useParams } from 'react-router-dom'
import {
  CheckSquare,
  Database,
  LayoutDashboard,
  MessageSquare,
  Plus,
  Settings,
  TerminalSquare,
  Workflow,
  Users,
  ChevronsUpDown,
  ChevronDown,
  ChevronRight,
  Folder,
  Check,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ProjectStatusBadge } from '@/components/layout/project-status-badge'
import { UlidChip } from '@/components/shared/ulid-chip'
import { useProjectQuery, useProjectsQuery } from '@/api/projects'
import type { ProjectSummary } from '@/api/types'
import { cn } from '@/lib/utils'

interface NavItem {
  to: string
  labelKey: 'overview' | 'tasks' | 'room' | 'memory' | 'artifacts' | 'settings'
  icon: typeof LayoutDashboard
}

interface SidebarProps {
  onCreateProject(): void
}

const NAV_ITEMS: ReadonlyArray<NavItem> = [
  { to: 'overview', labelKey: 'overview', icon: LayoutDashboard },
  { to: 'tasks', labelKey: 'tasks', icon: CheckSquare },
  { to: 'room', labelKey: 'room', icon: MessageSquare },
  { to: 'memory', labelKey: 'memory', icon: Database },
  { to: 'artifacts', labelKey: 'artifacts', icon: Workflow },
  { to: 'settings', labelKey: 'settings', icon: Settings },
]

const AGENT_SUB_ITEMS: ReadonlyArray<{ to: string; typeKey: 'claude-code' | 'codex' | 'gemini' }> =
  [
    { to: '/agents/claude', typeKey: 'claude-code' },
    { to: '/agents/codex', typeKey: 'codex' },
    { to: '/agents/gemini', typeKey: 'gemini' },
  ]

export function Sidebar({ onCreateProject }: SidebarProps) {
  const { t } = useTranslation()
  const params = useParams<{ projectId?: string }>()
  const navigate = useNavigate()
  const location = useLocation()
  const projectsQuery = useProjectsQuery()
  const projectDetailQuery = useProjectQuery(params.projectId)

  const projects = useMemo(() => projectsQuery.data?.items ?? [], [projectsQuery.data])
  const activeProject: ProjectSummary | undefined = useMemo(
    () => projects.find((project) => project.projectId === params.projectId),
    [projects, params.projectId],
  )
  const projectDetail = projectDetailQuery.data

  return (
    <aside className="z-20 flex h-screen w-64 shrink-0 flex-col border-r border-[hsl(var(--border))] bg-[hsl(var(--card))]">
      <div className="flex h-16 shrink-0 items-center gap-2 border-b border-[hsl(var(--border))] px-6">
        <TerminalSquare className="h-6 w-6 text-[hsl(var(--primary))]" />
        <span className="text-lg font-bold tracking-tight">
          Agent<span className="text-[hsl(var(--primary))]">Flow</span>
        </span>
      </div>

      <div className="flex flex-1 flex-col gap-6 overflow-y-auto overflow-x-hidden p-4">
        <Button onClick={onCreateProject} className="w-full justify-start gap-2 shadow-sm">
          <Plus className="h-4 w-4" /> {t('nav.newProject')}
        </Button>

        <section className="space-y-2">
          <p className="px-2 text-xs font-semibold uppercase tracking-wider text-[hsl(var(--muted-foreground))]">
            {t('nav.workspace')}
          </p>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                className="flex w-full items-center justify-between gap-2 rounded-lg border border-[hsl(var(--border))] bg-[hsl(var(--card))] p-3 text-left transition-colors hover:border-[hsl(var(--primary)/0.4)] hover:shadow-sm"
              >
                <div className="flex flex-col overflow-hidden">
                  <span className="truncate text-sm font-semibold text-[hsl(var(--foreground))]">
                    {activeProject?.name ?? t('nav.noProject')}
                  </span>
                  <span className="truncate font-mono text-xs text-[hsl(var(--muted-foreground))]">
                    {activeProject?.projectId ?? t('nav.pickFromTopbar')}
                  </span>
                </div>
                <ChevronsUpDown className="h-4 w-4 shrink-0 text-[hsl(var(--muted-foreground))]" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-72">
              <DropdownMenuLabel>{t('nav.switchProject')}</DropdownMenuLabel>
              {projects.map((project) => (
                <DropdownMenuItem
                  key={project.projectId}
                  selected={project.projectId === activeProject?.projectId}
                  onSelect={() => navigate(`/projects/${project.projectId}/tasks`)}
                >
                  <div className="flex items-center gap-2 overflow-hidden">
                    <Folder
                      className={cn(
                        'h-4 w-4 shrink-0',
                        project.projectId === activeProject?.projectId
                          ? 'text-[hsl(var(--primary))]'
                          : 'text-[hsl(var(--muted-foreground))]',
                      )}
                    />
                    <div className="flex flex-col overflow-hidden">
                      <span className="truncate text-sm font-medium text-[hsl(var(--foreground))]">
                        {project.name}
                      </span>
                      <span className="truncate font-mono text-xs text-[hsl(var(--muted-foreground))]">
                        {project.projectId}
                      </span>
                    </div>
                  </div>
                </DropdownMenuItem>
              ))}
              {projects.length === 0 ? (
                <p className="px-2 py-3 text-center text-xs text-[hsl(var(--muted-foreground))]">
                  {t('nav.emptyProjects', { defaultValue: '暂无项目，先创建一个' })}
                </p>
              ) : null}
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={onCreateProject}>
                <Plus className="h-4 w-4 text-[hsl(var(--primary))]" /> {t('nav.newProject')}…
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => navigate('/projects')}>
                <Check className="h-4 w-4 text-[hsl(var(--muted-foreground))]" />{' '}
                {t('nav.viewAllProjects', { defaultValue: '查看全部项目' })}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>

          {activeProject ? (
            <div className="rounded-md border border-dashed border-[hsl(var(--border))] bg-[hsl(var(--muted))] p-2 text-xs text-[hsl(var(--muted-foreground))]">
              <div className="flex items-center justify-between">
                <span>{t('nav.projectStatus')}</span>
                <ProjectStatusBadge status={activeProject.status} />
              </div>
              <div className="mt-1 flex items-center justify-between">
                <span>{t('nav.progress')}</span>
                <span className="font-mono">
                  {activeProject.progress.done}/{activeProject.progress.total}
                </span>
              </div>
            </div>
          ) : null}

          {projectDetail ? (
            <div className="space-y-2 rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--card))] p-2 text-xs text-[hsl(var(--muted-foreground))]">
              {projectDetail.goal ? (
                <p className="break-words text-[hsl(var(--foreground))]">{projectDetail.goal}</p>
              ) : null}
              <div className="min-w-0 space-y-1.5">
                <div className="min-w-0 flex flex-wrap items-start gap-1.5">
                  <span className="shrink-0 pt-0.5">项目 ID</span>
                  <UlidChip className="min-w-0 flex-1" id={projectDetail.projectId} full={false} />
                </div>
                <div className="min-w-0 flex flex-wrap items-start gap-1.5">
                  <span className="shrink-0 pt-0.5">会话 ID</span>
                  <UlidChip
                    className="min-w-0 flex-1"
                    id={projectDetail.activeSessionId}
                    full={false}
                  />
                </div>
                <div className="min-w-0 flex flex-wrap items-start gap-1.5">
                  <span className="shrink-0 pt-0.5">聊天室 ID</span>
                  <UlidChip className="min-w-0 flex-1" id={projectDetail.roomId} full={false} />
                </div>
                <div className="space-y-1">
                  <span>OpenViking</span>
                  <p className="break-all font-mono text-[10px] leading-snug">
                    {projectDetail.openvikingRoot}
                  </p>
                </div>
              </div>
            </div>
          ) : null}
        </section>

        <nav className="space-y-1">
          {!activeProject ? <AgentsNavGroup defaultOpen pathname={location.pathname} /> : null}

          {!activeProject ? (
            <NavLink
              to="/settings"
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                  isActive
                    ? 'bg-[hsl(var(--accent))] text-[hsl(var(--accent-foreground))]'
                    : 'text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--muted))] hover:text-[hsl(var(--foreground))]',
                )
              }
            >
              <Settings className="h-4 w-4" />
              <span className="flex-1">
                {t('nav.systemSettings', { defaultValue: '系统设置' })}
              </span>
            </NavLink>
          ) : null}

          {activeProject
            ? NAV_ITEMS.map((item) => (
                <NavLink
                  key={item.to}
                  to={`/projects/${activeProject.projectId}/${item.to}`}
                  className={({ isActive }) =>
                    cn(
                      'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                      isActive
                        ? 'bg-[hsl(var(--accent))] text-[hsl(var(--accent-foreground))]'
                        : 'text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--muted))] hover:text-[hsl(var(--foreground))]',
                    )
                  }
                >
                  <item.icon className="h-4 w-4" />
                  <span className="flex-1">{t(`nav.${item.labelKey}`)}</span>
                </NavLink>
              ))
            : null}
        </nav>
      </div>
    </aside>
  )
}

interface AgentsNavGroupProps {
  defaultOpen?: boolean
  pathname: string
}

function AgentsNavGroup({ defaultOpen = true, pathname }: AgentsNavGroupProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(defaultOpen)
  const groupActive = pathname === '/agents' || pathname.startsWith('/agents/')

  return (
    <div className="space-y-1">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className={cn(
          'flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
          groupActive
            ? 'bg-[hsl(var(--accent))] text-[hsl(var(--accent-foreground))]'
            : 'text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--muted))] hover:text-[hsl(var(--foreground))]',
        )}
      >
        <Users className="h-4 w-4" />
        <span className="flex-1 text-left">{t('nav.agents')}</span>
        {open ? (
          <ChevronDown className="h-4 w-4 shrink-0 opacity-70" />
        ) : (
          <ChevronRight className="h-4 w-4 shrink-0 opacity-70" />
        )}
      </button>

      {open ? (
        <div className="ml-4 space-y-0.5 border-l border-[hsl(var(--border))] pl-3">
          {AGENT_SUB_ITEMS.map((sub) => (
            <NavLink
              key={sub.to}
              to={sub.to}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-2 rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
                  isActive
                    ? 'bg-[hsl(var(--accent))] text-[hsl(var(--accent-foreground))]'
                    : 'text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--muted))] hover:text-[hsl(var(--foreground))]',
                )
              }
            >
              <span className="flex-1">{t(`agent.type.${sub.typeKey}`)}</span>
            </NavLink>
          ))}
        </div>
      ) : null}
    </div>
  )
}
