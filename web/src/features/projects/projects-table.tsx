import { useNavigate } from 'react-router-dom'
import { motion } from 'framer-motion'
import { ArrowRight, Folder } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { ProjectStatusBadge } from '@/components/layout/project-status-badge'
import { UlidChip } from '@/components/shared/ulid-chip'
import { EmptyState } from '@/components/shared/empty-state'
import { formatRelativeTime } from '@/lib/format'
import { fadeUp, stagger } from '@/components/shared/motion-presets'
import type { ProjectSummary } from '@/api/types'

interface ProjectsTableProps {
  projects: ProjectSummary[]
  isLoading: boolean
  onCreate(): void
}

export function ProjectsTable({ projects, isLoading, onCreate }: ProjectsTableProps) {
  const navigate = useNavigate()

  if (!isLoading && projects.length === 0) {
    return (
      <EmptyState
        icon={<Folder className="h-8 w-8" />}
        title="还没有项目"
        description="创建第一个项目，把它委托给 Claude / Codex / Gemini。"
        action={
          <button
            onClick={onCreate}
            className="rounded-md bg-[hsl(var(--primary))] px-4 py-2 text-sm font-medium text-[hsl(var(--primary-foreground))] hover:bg-[hsl(var(--primary)/0.9)]"
          >
            新建项目
          </button>
        }
      />
    )
  }

  return (
    <motion.ul
      variants={stagger}
      initial="hidden"
      animate="visible"
      className="grid gap-3 md:grid-cols-2 xl:grid-cols-3"
    >
      {projects.map((project) => (
        <motion.li key={project.projectId} variants={fadeUp}>
          <Card
            className="group cursor-pointer transition-all hover:border-[hsl(var(--primary)/0.4)] hover:shadow-md"
            onClick={() => navigate(`/projects/${project.projectId}/tasks`)}
          >
            <CardContent className="space-y-3 p-5">
              <div className="flex items-start justify-between gap-3">
                <div className="space-y-1 overflow-hidden">
                  <p className="truncate text-base font-semibold text-[hsl(var(--foreground))]">
                    {project.name}
                  </p>
                  <UlidChip id={project.projectId} />
                </div>
                <ProjectStatusBadge status={project.status} />
              </div>
              <div className="flex items-center justify-between text-xs text-[hsl(var(--muted-foreground))]">
                <span>
                  任务{' '}
                  <span className="font-mono text-[hsl(var(--foreground))]">
                    {project.progress.done}
                  </span>
                  /<span className="font-mono">{project.progress.total}</span>
                  {project.progress.blocked > 0 ? (
                    <span className="ml-2 text-orange-600">阻塞 {project.progress.blocked}</span>
                  ) : null}
                </span>
                <span>更新于 {formatRelativeTime(project.updatedAt)}</span>
              </div>
              <div className="flex items-center justify-end text-xs font-medium text-[hsl(var(--primary))] opacity-0 transition-opacity group-hover:opacity-100">
                进入项目 <ArrowRight className="ml-1 h-3.5 w-3.5" />
              </div>
            </CardContent>
          </Card>
        </motion.li>
      ))}
      {isLoading
        ? Array.from({ length: 3 }).map((_, idx) => (
            <li key={`skeleton-${idx}`}>
              <Card>
                <CardContent className="space-y-3 p-5">
                  <div className="h-4 w-2/3 animate-pulse rounded bg-[hsl(var(--muted))]" />
                  <div className="h-3 w-1/3 animate-pulse rounded bg-[hsl(var(--muted))]" />
                  <div className="h-2 w-full animate-pulse rounded bg-[hsl(var(--muted))]" />
                </CardContent>
              </Card>
            </li>
          ))
        : null}
    </motion.ul>
  )
}
